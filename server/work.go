package server

import (
  "context"
  "fmt"
  "io"
  "io/ioutil"
  "k8s.io/apimachinery/pkg/api/resource"
  "os"
  "path"
  "path/filepath"
  "strings"
  "time"

  "bytes"

  "github.com/Unknwon/com"
  azaws "github.com/aws/aws-sdk-go/aws"
  "github.com/fatih/color"
  "github.com/pkg/errors"
  "github.com/rai-project/archive"
  "github.com/rai-project/aws"
  "github.com/rai-project/config"
  "github.com/rai-project/docker"
  "github.com/rai-project/model"
  "github.com/rai-project/pubsub"
  "github.com/rai-project/pubsub/redis"
  "github.com/rai-project/store"
  "github.com/rai-project/store/s3"
  "github.com/rai-project/uuid"

  "archive/tar"
  shellwords "github.com/junegunn/go-shellwords"
  corev1 "k8s.io/api/core/v1"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  "k8s.io/apimachinery/pkg/runtime"
  "k8s.io/client-go/kubernetes"
  "k8s.io/client-go/rest"
  "k8s.io/client-go/tools/remotecommand"
)

type WorkRequest struct {
	*model.JobRequest
	publisher      pubsub.Publisher
	publishChannel string
	pubsubConn     pubsub.Connection
	docker         *docker.Client
	container      *docker.Container
	buildSpec      model.BuildSpecification
	store          store.Store
	stdout         io.Writer
	stderr         io.Writer
	canceler       context.CancelFunc
	serverOptions  Options
}

type publishWriter struct {
	publisher      pubsub.Publisher
	publishChannel string
	kind           model.ResponseKind
}

func (w *publishWriter) Write(p []byte) (int, error) {
	w.publisher.Publish(w.publishChannel, model.JobResponse{
		Kind:      w.kind,
		Body:      p,
		CreatedAt: time.Now(),
	})
	return len(p), nil
}

var (
	DefaultUploadExpiration = func() time.Time {
		return time.Now().AddDate(0, 1, 0) // next month
	}
)

// NewWorkerRequest ...
func NewWorkerRequest(job *model.JobRequest, serverOpts Options) (*WorkRequest, error) {
	publishChannel := serverOpts.clientAppName + "/log-" + job.ID.Hex()

	conn, err := redis.New()
	if err != nil {
		return nil, err
	}
	publisher, err := redis.NewPublisher(conn)
	if err != nil {
		return nil, err
	}

	stdout := &publishWriter{
		publisher:      publisher,
		publishChannel: publishChannel,
		kind:           model.StdoutResponse,
	}

	stderr := &publishWriter{
		publisher:      publisher,
		publishChannel: publishChannel,
		kind:           model.StderrResponse,
	}

	session, err := aws.NewSession(
		aws.Region(aws.AWSRegionUSEast1),
		aws.AccessKey(aws.Config.AccessKey),
		aws.SecretKey(aws.Config.SecretKey),
	)
	if err != nil {
		return nil, err
	}
	st, err := s3.New(
		s3.Session(session),
		store.Bucket(serverOpts.clientUploadBucketName),
	)
	if err != nil {
		return nil, err
	}

	var canceler context.CancelFunc
	if serverOpts.timelimit != 0 {
		serverOpts.context, canceler = context.WithTimeout(serverOpts.context, serverOpts.timelimit)
	} else {
		serverOpts.context, canceler = context.WithCancel(serverOpts.context)
	}

	d, err := docker.NewClient(
		docker.ClientContext(serverOpts.context),
		docker.Stdout(stdout),
		docker.Stderr(stderr),
		docker.Stdin(nil),
	)
	if err != nil {
		return nil, err
	}

	return &WorkRequest{
		JobRequest:     job,
		pubsubConn:     conn,
		publishChannel: publishChannel,
		publisher:      publisher,
		docker:         d,
		buildSpec:      job.BuildSpecification,
		store:          st,
		canceler:       canceler,
		serverOptions:  serverOpts,
	}, nil
}

func (w *WorkRequest) publishStdout(s string) error {
	return w.publisher.Publish(w.publishChannel, model.JobResponse{
		ID:        uuid.NewV4(),
		Kind:      model.StdoutResponse,
		Body:      []byte(s),
		CreatedAt: time.Now(),
	})
}

func (w *WorkRequest) publishStderr(s string) error {
	return w.publisher.Publish(w.publishChannel, model.JobResponse{
		ID:        uuid.NewV4(),
		Kind:      model.StderrResponse,
		Body:      []byte(s),
		CreatedAt: time.Now(),
	})
}

func (w *WorkRequest) buildImage(spec *model.BuildImageSpecification, uploadedReader io.Reader) error {
	if spec == nil {
		return nil
	}

	if spec.ImageName == "" {
		spec.ImageName = uuid.NewV4()
	}

	if !Config.DisableRAIDockerNamespaceProtection {
		appName := strings.TrimSuffix(config.App.Name, "d")
		if strings.HasPrefix(spec.ImageName, appName) || strings.HasPrefix(spec.ImageName, config.App.Name) {
			w.publishStderr(color.RedString("✱ Docker image name cannot start with " + appName + "/ . Choose a different prefix."))
			return errors.New("docker image namespace")
		}
	}

	if w.docker.HasImage(spec.ImageName) && spec.NoCache == false {
		w.publishStdout(color.YellowString("✱ Using cached version of the docker image. Set no_cache=true to disable cache."))
		return nil
	}

	tmpDir, err := ioutil.TempDir(config.App.TempDir, "buildImage")
	if err != nil {
		w.publishStderr(color.RedString("✱ Server was unable to create a temporary directory."))
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := archive.Unzip(uploadedReader, tmpDir); err != nil {
		w.publishStderr(color.RedString("✱ Unable to unzip your folder " + err.Error() + "."))
		return err
	}

	if spec.Dockerfile == "" {
		spec.Dockerfile = "Dockerfile"
	}

	dockerfile := filepath.Join(tmpDir, spec.Dockerfile)
	if !com.IsFile(dockerfile) {
		w.publishStderr(color.RedString("✱ Unable to find Dockerfile. Make sure the path is specified correctly."))
		return errors.Errorf("file %v not found", dockerfile)
	}

	f, err := archive.Zip(tmpDir)
	if err != nil {
		w.publishStderr(color.RedString("✱ Unable to create archive of uploaded directory."))
		return err
	}
	defer f.Close()

	w.publishStdout(color.YellowString("✱ Server is starting to build image."))
	if err := w.docker.ImageBuild(spec.ImageName, spec.Dockerfile, f); err != nil {
		w.publishStderr(color.RedString("✱ Unable to build dockerfile."))
		return err
	}

	w.publishStdout(color.YellowString("✱ Server has build your " + spec.ImageName + " docker image."))
	return nil
}

func (w *WorkRequest) pushImage(buildSpec *model.BuildImageSpecification, uploadedReader io.Reader) error {
	if !buildSpec.PushQ() {
		return nil
	}

	pushSpec := buildSpec.Push

	if !w.docker.HasImage(pushSpec.ImageName) {
		w.publishStdout(color.YellowString("✱ Unable to find " + pushSpec.ImageName +
			". Make sure you have built the image with the same name as the one being published."))
		return errors.Errorf("image %s found", pushSpec.ImageName)
	}
	err := w.docker.ImagePush(pushSpec.ImageName, *pushSpec)
	if err != nil {
		w.publishStdout(color.YellowString("✱ Unable to push " + pushSpec.ImageName +
			" to docker registry."))
		return errors.Wrapf(err, "unable to push %s to docker registry", pushSpec.ImageName)
	}

	return nil
}

func (w *WorkRequest) Start() error {
	ctx := w.serverOptions.context
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.run()
	}()

	select {
	case <-ctx.Done():
		err := ctx.Err()
		if err == nil {
			w.publishStderr(color.RedString("✱ The server has terminated your job since it exceeds the configured time limit."))
			return nil
		}
		return err
	case err := <-errChan:
		return err
	}
}

func (w *WorkRequest) run() error {
	buildSpec := w.buildSpec

	defer func() {
		if r := recover(); r != nil {
			w.publishStderr(color.RedString("✱ Server crashed while processing your request."))
		}
	}()

	defer func() {
		w.publishStdout(color.GreenString("✱ Server has ended your request."))
		w.publisher.End(w.publishChannel)
	}()

	w.publishStdout(color.YellowString("✱ Server has accepted your job submission and started to configure the container."))

	w.publishStdout(color.YellowString("✱ Downloading your code."))

	buf := new(azaws.WriteAtBuffer)
	if err := w.store.DownloadTo(buf, w.UploadKey); err != nil {
		w.publishStderr(color.RedString("✱ Failed to download your code."))
		log.WithError(err).WithField("key", w.UploadKey).Error("failed to download user code from store")
		return err
	}

	//err := w.buildImage(buildSpec.Commands.BuildImage, bytes.NewBuffer(buf.Bytes()))
	//if err != nil {
	//	w.publishStderr(color.RedString("✱ Unable to create image " + buildSpec.Commands.BuildImage.ImageName + "."))
	//	return err
	//} else if buildSpec.Commands.BuildImage != nil {
	//	buildSpec.RAI.ContainerImage = buildSpec.Commands.BuildImage.ImageName
	//}
  //
	//err = w.pushImage(buildSpec.Commands.BuildImage, bytes.NewBuffer(buf.Bytes()))
	//if err != nil {
	//	w.publishStderr(color.RedString("✱ Unable to push image " + buildSpec.Commands.BuildImage.Push.ImageName + "."))
	//	return err
	//}
  //
	imageName := buildSpec.RAI.ContainerImage
	//w.publishStdout(color.YellowString("✱ Using " + imageName + " as container image."))
  //
	//if buildSpec.Commands.BuildImage == nil && !w.docker.HasImage(imageName) {
	//	log.WithField("id", w.ID).WithField("image", imageName).Debug("image not found")
	//	err := w.docker.PullImage(imageName)
	//	if err != nil {
	//		w.publishStderr(color.RedString("✱ Unable to pull " + imageName + " from docker hub repository."))
	//		log.WithError(err).WithField("image", imageName).Error("unable to pull image")
	//		return err
	//	}
	//}
	// there is nothing to build...
	if len(buildSpec.Commands.Build) == 0 {
		return nil
	}

	srcDir := w.serverOptions.containerSourceDirectory
	buildDir := w.serverOptions.containerBuildDirectory

  // create w.pod and start it

  var cmds []string
  cmds = []string{"sh", "-c", "sleep infinity"}
  podClient := w.serverOptions.k8sClientset.CoreV1().Pods("task-build")
  podName := w.ID.Hex()
  pod := &corev1.Pod{
    ObjectMeta: metav1.ObjectMeta{
      Name: podName,
      Namespace: "task-build",
    },
    Spec: corev1.PodSpec{
      Containers: []corev1.Container{
        {
          Name:  "ece408",
          Image: imageName,
          Command: cmds,
          WorkingDir: buildDir,
          VolumeMounts: []corev1.VolumeMount{
            {
              Name: "build-dir",
              MountPath: "/build",
            },
          },
          Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1"),}},
        },
      },
      Volumes: []corev1.Volume{
        {        Name: "build-dir",
          VolumeSource: corev1.VolumeSource{
            EmptyDir:              nil,
          }},
      },
    },
  }
  _, err := podClient.Create(context.TODO(), pod, metav1.CreateOptions{})
  if err != nil{
    log.Fatal(err)
  }
  pod, err = podClient.Get(context.TODO(), podName, metav1.GetOptions{})
  if err != nil{
    log.Fatal(err)
  }
  for pod.Status.Phase != "Running"{
    pod, err = podClient.Get(context.TODO(), podName, metav1.GetOptions{})
    if err != nil{
      log.Fatal(err)
    }
    fmt.Println(pod.Status.Phase)
  }
  tmpDir, err := ioutil.TempDir("./", "buildImage-Pod")  // @Fixme: "config.App.TempDir" ../server/work.go:427:39: config.App undefined (type *rest.Config has no field or method App)
  if err != nil {
    w.publishStderr(color.RedString("✱ Server was unable to create a temporary directory."))
    return err
  }
  defer os.RemoveAll(tmpDir)
  if err := archive.Unzip(bytes.NewBuffer(buf.Bytes()), tmpDir); err != nil {
    w.publishStderr(color.RedString("✱ Unable to unzip your folder " + err.Error() + "."))
    return err
  }
  err = copyToPod(w.serverOptions.k8sClientset, w.serverOptions.k8sRestConfig, podName, "task-build", tmpDir, srcDir)
  if err != nil{
    log.Fatal(err)
  }

  defer func() {
    fmt.Println("copying back")
   opts := w.serverOptions
   dir := opts.containerBuildDirectory
   fmt.Println(dir)
   r, err := copyFromPod(w.serverOptions.k8sClientset, w.serverOptions.k8sRestConfig, podName, "task-build", dir, "")
   if err != nil {
     w.publishStderr(color.RedString("✱ Unable to copy your output data in " + dir + " from the container."))
     log.WithError(err).WithField("dir", dir).Error("unable to get user data from container")
     return
   }
   	uploadKey := opts.clientUploadDestinationDirectory + "/build-" + w.ID.Hex() + "." + archive.Extension()
   	key, err := w.store.UploadFrom(
   		r,
   		uploadKey,
   		s3.Expiration(DefaultUploadExpiration()),
   		s3.Metadata(map[string]interface{}{
   			"id":           w.ID,
   			"type":         "server_upload",
   			"worker":       info(),
   			"profile":      w.User,
   			"submitted_at": w.CreatedAt,
   			"created_at":   time.Now(),
   		}),
   		s3.ContentType(archive.MimeType()),
   	)
   	if err != nil {
   		w.publishStderr(color.RedString("✱ Unable to upload your output data in " + dir + " to the store server."))
   		log.WithError(err).WithField("dir", dir).WithField("key", uploadKey).Error("unable to upload user data to store")
   		return
   	}
   	w.publishStdout(color.GreenString(
   		"✱ The build folder has been uploaded to " + key +
   			". The data will be present for only a short duration of time.",
   	))
    err = podClient.Delete(context.TODO(), podName, metav1.DeleteOptions{})
    if err != nil{
      log.Fatal(err)
    }
  }()
	for _, cmd := range buildSpec.Commands.Build {
    w.publishStdout(color.GreenString("✱ Running " + cmd))
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		args, err := shellwords.Parse(cmd)
    if err != nil{
      fmt.Println("error")
      panic(err.Error())
    }
		err = execToPodThroughAPI(w.serverOptions.k8sClientset, w.serverOptions.k8sRestConfig, args, "", podName, "task-build", nil, os.Stdout, os.Stderr)
		if err != nil{
      fmt.Println("error")
      panic(err.Error())
    }
		w.publishStdout(color.GreenString("✱ Finished " + cmd))
	}
	return nil
}

func (w *WorkRequest) Close() error {

	if w.container != nil {
		w.container.Close()
	}

	if w.docker != nil {
		w.docker.Close()
	}

	if w.publisher != nil {
		if err := w.publisher.Stop(); err != nil {
			log.WithError(err).Error("failed to stop pubsub publisher")
		}
	}

	if w.pubsubConn != nil {
		if err := w.pubsubConn.Close(); err != nil {
			log.WithError(err).Error("failed to close pubsub connection")
		}
	}

	for _, f := range w.serverOptions.onWorkerClose {
		f()
	}

	if w.canceler != nil {
		w.canceler()
	}

	return nil
}

func execToPodThroughAPI(clientset *kubernetes.Clientset, restConfig *rest.Config, command []string, containerName, podName, namespace string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (error) {
  req := clientset.CoreV1().RESTClient().Post().
    Resource("pods").
    Name(podName).
    Namespace(namespace).
    SubResource("exec")
  scheme := runtime.NewScheme()
  if err := corev1.AddToScheme(scheme); err != nil {
    return fmt.Errorf("error adding to scheme: %v", err)
  }

  parameterCodec := runtime.NewParameterCodec(scheme)
  req.VersionedParams(&corev1.PodExecOptions{
    Command:   command,
    Container: containerName,
    Stdin:     stdin != nil,
    Stdout:    true,
    Stderr:    true,
    TTY:       false,   // fixme
  }, parameterCodec)

  exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
  if err != nil {
    return fmt.Errorf("error while creating Executor: %v", err)
  }

  err = exec.Stream(remotecommand.StreamOptions{
    Stdin:  stdin,
    Stdout: stdout,
    Stderr: stderr,
    Tty:    false,
  })
  if err != nil {
    return fmt.Errorf("error in Stream: %v", err)
  }

  return nil
}
func cpMakeTar(srcPath, destPath string, writer io.Writer) error{
  tarWriter := tar.NewWriter(writer)
  defer tarWriter.Close()

  srcPath = path.Clean(srcPath)
  destPath = path.Clean(destPath)
  return recursiveTar(path.Dir(srcPath), path.Base(srcPath), path.Dir(destPath), path.Base(destPath), tarWriter)
}
func recursiveTar(srcBase, srcFile, destBase, destFile string, tw *tar.Writer) error {
  filepath := path.Join(srcBase, srcFile)
  stat, err := os.Lstat(filepath)
  if err != nil {
    return err
  }
  if stat.IsDir() {
    files, err := ioutil.ReadDir(filepath)
    if err != nil {
      return err
    }
    if len(files) == 0 {
      //case empty directory
      hdr, _ := tar.FileInfoHeader(stat, filepath)
      hdr.Name = destFile
      if err := tw.WriteHeader(hdr); err != nil {
        return err
      }
    }
    for _, f := range files {
      if err := recursiveTar(srcBase, path.Join(srcFile, f.Name()), destBase, path.Join(destFile, f.Name()), tw); err != nil {
        return err
      }
    }
    return nil
  } else if stat.Mode()&os.ModeSymlink != 0 {
    //case soft link
    hdr, _ := tar.FileInfoHeader(stat, filepath)
    target, err := os.Readlink(filepath)
    if err != nil {
      return err
    }

    hdr.Linkname = target
    hdr.Name = destFile
    if err := tw.WriteHeader(hdr); err != nil {
      return err
    }
  } else {
    //case regular file or other file type like pipe
    hdr, err := tar.FileInfoHeader(stat, filepath)
    if err != nil {
      return err
    }
    hdr.Name = destFile

    if err := tw.WriteHeader(hdr); err != nil {
      return err
    }

    f, err := os.Open(filepath)
    if err != nil {
      return err
    }
    defer f.Close()

    if _, err := io.Copy(tw, f); err != nil {
      return err
    }
    return f.Close()
  }
  return nil
}
// #2 copyToPod
func copyToPod(clientset *kubernetes.Clientset, restConfig *rest.Config, podName, namespace, srcPath, destPath string) (error) {
  reader, writer := io.Pipe()
  if destPath != "/" && strings.HasSuffix(string(destPath[len(destPath)-1]), "/") {
    destPath = destPath[:len(destPath)-1]
  }
  //if err := checkDestinationIsDir(i, destPath); err == nil {
  //destPath = destPath + "/" + path.Base(srcPath)
  //}
  go func() {
    defer writer.Close()
    err := cpMakeTar(srcPath, destPath, writer)
    if err != nil{
      panic(err.Error())
    }
  }()
  var cmdArr []string

  cmdArr = []string{"tar", "-xf", "-"}  // extract from Pipe
  destDir := path.Dir(destPath)
  println(destDir)
  if len(destDir) > 0 {
    cmdArr = append(cmdArr, "-C", destDir)
  }
  return execToPodThroughAPI(clientset, restConfig, cmdArr, "", podName, namespace, reader, os.Stdout, os.Stderr)
}

func copyFromPod(clientset *kubernetes.Clientset, restConfig *rest.Config, podName, namespace, srcPath, destPath string) (io.Reader, error) {
  if srcPath != "/" && strings.HasSuffix(string(srcPath[len(srcPath)-1]), "/") {
    srcPath = srcPath[:len(srcPath)-1]
  }
  reader, writer := io.Pipe()  // tar result avail at reader
  cmdArr := []string{"tar", "-cf", "-", srcPath}  // create from Pipe
  go func(){
    defer writer.Close()
    err := execToPodThroughAPI(clientset, restConfig, cmdArr, "", podName, namespace, nil, writer, os.Stderr)
    if err != nil{
      panic(err.Error())
    }
  }()
  return reader, nil
}
