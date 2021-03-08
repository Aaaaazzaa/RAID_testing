package main

import (
	"archive/tar"
	"io/ioutil"

	//"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	//"k8s.io/apimachinery/pkg/runtime"
	//"k8s.io/client-go/tools/clientcmd"
	//"k8s.io/client-go/tools/remotecommand"
	//"path/filepath"
	//"strings"

	//"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/homedir"
	//"os"
	"path"
	"k8s.io/client-go/kubernetes"
	//"k8s.io/client-go/kubernetes"
	"os"
	"path/filepath"
	"strings"
	//"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/apimachinery/pkg/runtime"
	//"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	//"k8s.io/kubernetes/pkg/kubectl/cmd/"
	//_ "k8s.io/kubernetes/pkg/kubectl/cmd/cp"
	//cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	//_ "unsafe"
)

// ExecCmd exec command on specific pod and wait the command's output.
//func ExecCmdExample(client kubernetes.Interface, config *restclient.Config, podName string,
//	command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
//	cmd := []string{
//		"sh",
//		"-c",
//		command,
//	}
//	req := client.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
//		Namespace("default").SubResource("exec")
//	option := &v1.PodExecOptions{
//		Command: cmd,
//		Stdin:   true,
//		Stdout:  true,
//		Stderr:  true,
//		TTY:     true,
//	}
//	if stdin == nil {
//		option.Stdin = false
//	}
//	req.VersionedParams(
//		option,
//		scheme.ParameterCodec,
//	)
//	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
//	if err != nil {
//		return err
//	}
//	err = exec.Stream(remotecommand.StreamOptions{
//		Stdin:  stdin,
//		Stdout: stdout,
//		Stderr: stderr,
//	})
//	if err != nil {
//		return err
//	}
//
//	return nil
//}

// #1 pod exec
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
		TTY:       false,
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

	cmdArr = []string{"tar", "-xf", "-"}
	destDir := path.Dir(destPath)
	if len(destDir) > 0 {
		cmdArr = append(cmdArr, "-C", destDir)
	}
	return execToPodThroughAPI(clientset, restConfig, cmdArr, "", podName, namespace, reader, os.Stdout, os.Stderr)
}

func main() {
	// create pod
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		err = fmt.Errorf("failed creating clientset. Error: %+v", err)
		panic(err)
	}

	var cmds []string
	cmds = []string{"sh", "-c", "echo 'Hello, Kubernetes!'"}
	podClient := clientset.CoreV1().Pods("default")
	podName := "demo-job-146"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "ece408",
					Image: "nginx:1.12",
					Command: cmds,
				},
			},
		},
	}
	_, err = podClient.Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil{
		panic(err.Error())
	}
	pod, err = podClient.Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil{
		panic(err.Error())
	}
	for pod.Status.Phase != "Running"{
		pod, err = podClient.Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil{
			panic(err.Error())
		}
		fmt.Println(pod.Status.Phase)
	}
	//fmt.Println(podResult)
	//fmt.Println(podResult.Status)
	//for true{
	//pod, err = podClient.Get(context.TODO(), podName, metav1.GetOptions{})
	//if err != nil{
	//	panic(err.Error())
	//}
	//fmt.Println(podResult.Status)
	//}

	err = copyToPod(clientset, config, podName, "default", "./hello_src", "hello_dst")
	if err != nil{
		panic(err.Error())
	}
	var cmdArr []string
	cmdArr = []string{"ls"}
	err = execToPodThroughAPI(clientset, config, cmdArr, "", podName, "default", os.Stdin, os.Stdout, os.Stderr)
	if err != nil{
		panic(err.Error())
	}
	//os.Create("testDir-findme")
	//var stdout, stderr bytes.Buffer
	//err := execToPodThroughAPI("ls", "", "busybox-test", "default", nil, &stdout, &stderr)
	//if err != nil{
	//	panic(err.Error())
	//}
	//fmt.Println(stderr.String())
	//fmt.Println(stdout.String())

	//var kubeconfig *string
	//if home := homedir.HomeDir(); home != "" {
	//	kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	//} else {
	//	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	//}
	//flag.Parse()
	//config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	//if err != nil {
	//	panic(err)
	//}
	//clientset, err := kubernetes.NewForConfig(config)
	//if err != nil {
	//	panic(err)
	//}
	//jobsClient := clientset.BatchV1().Jobs("default")
	//job := &batchv1.Job{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name: "demo-job",
	//		Namespace: "default",
	//	},
	//	Spec: batchv1.JobSpec{
	//		Template: apiv1.PodTemplateSpec{
	//			Spec: apiv1.PodSpec{
	//				Containers: []apiv1.Container{
	//					{	Name:  "web",
	//						Image: "nginx:1.12",
	//					},
	//				},
	//				RestartPolicy: apiv1.RestartPolicyNever,
	//			},
	//		},
	//	},
	//}
	//
	//fmt.Println("Creating job... ")
	//result1, err1 := jobsClient.Create(context.TODO(), job, metav1.CreateOptions{})
	//if err1 != nil {
	//	fmt.Println(err1)
	//	panic(err1)
	//}
	//fmt.Printf("Created job %q.\n", result1ult1)


	//var kubeconfig *string
	//if home := homedir.HomeDir(); home != "" {
	//	kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	//} else {
	//	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	//}
	//flag.Parse()
	//
	//config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	//if err != nil {
	//	panic(err)
	//}
	//restConfi, err := restclient.RESTClientFor(config)
	//clientset, err := kubernetes.NewForConfig(config)
	//if err != nil {
	//	panic(err)
	//}
	//api := clientset.AppsV1()
	////restconfig := &restclient.Config{Host: config.Host}
	//ExecCmdExample(api, config, "test", "echo hello", io.Reader(), io.Writer(), io.Writer())
	//var ns, label, field string
	//c, err := config.LoadKubeConfig()
	//if err != nil {
	//	panic(err.Error())
	//}
	//clientset := client.NewAPIClient(c)
	//
	//conf := &restclient.Config{
	//	Host:                "https://127.0.0.1:32768",
	//}
	//restclient.RESTClientFor(conf)
	////conf.ContentConfig.GroupVersion =
	//restClient, err := restclient.RESTClientFor(conf)
	//if err != nil{
	//	panic(err.Error())
	//}
	////req = restClient.Post().Resource("pods").Name("busybox-test").Namespace("default").SubResource("exec").Param("con")
	//kubernetes.Interface.CoreV1()

	//arr := "cat test.txt; echo This special message goes to stderr > test.txt; echo This message goes to stdout"
	//s, resp, err := clientset.CoreV1Api.ConnectGetNamespacedPodExec(context.TODO(), "busybox-test", "default",
	//	map[string]interface{}{"command": arr, "stderr": false, "stdin": false, "stdout": true, "tty": false})
	//if err != nil{
	//	panic(err.Error())
	//}
	//fmt.Println(s)
	//fmt.Print(resp)
}
