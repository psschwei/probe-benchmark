package podspeed

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	statistics "github.com/montanaflynn/stats"
	"github.com/psschwei/podspeed/pkg/pod"
	podtemplate "github.com/psschwei/podspeed/pkg/pod/template"
	podtypes "github.com/psschwei/podspeed/pkg/pod/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	clientappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/tools/clientcmd"

	// Allow podspeed to run against a GCP cluster
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func Run(ns string, podObj *corev1.Pod, typ, template string, podN int, skipDelete, prepull, probe, details bool) {
	supportedTypes, err := podtypes.Names()
	if err != nil {
		log.Fatalln("failed to built in types: ", err)
	}

	if typ == "" {
		typ = "basic"
	}

	podFn, err := podtypes.GetConstructor(typ)
	if err != nil {
		log.Fatalln("failed to load constructor, valid values for -typ are: ", supportedTypes, err)
	}

	if template != "" {
		var content io.Reader
		if template == "-" {
			content = os.Stdin
		} else {
			file, err := os.Open(template)
			if err != nil {
				log.Fatalln("Failed to open template file", err)
			}
			content = file
		}
		fn, err := podtemplate.PodConstructorFromYAML(content)
		if err != nil {
			log.Fatalln("Failed to generate template from file", err)
		}
		podFn = fn
	}

	// skip if podObj is empty
	if !reflect.DeepEqual(podObj, &corev1.Pod{}) {
		fmt.Println("setting podFn for pobObj")
		fn, err := podtemplate.PodConstructorFromObject(podObj)
		if err != nil {
			log.Fatalln("Failed to generate pod object from file", err)
		}
		podFn = fn
	}

	if podN < 1 {
		log.Fatalln("-pods must not be smaller than 1")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load kubernetes config.
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
	if err != nil {
		log.Fatalln("Failed to load config", err)
	}

	// Create kubernetes client.
	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln("Failed to create Kubernetes client", err)
	}

	if prepull {
		log.Println("Prepulling images to all nodes")
		if err := prepullImages(ctx, kube.AppsV1().DaemonSets(ns), podFn(ns, "").Spec); err != nil {
			log.Fatalln("Failed to prepull images", err)
		}
		log.Println("Prepulling done")
	}

	runLabels := labels.Set{
		"podspeed/run": uuid.NewString(),
	}
	watcher, err := kube.CoreV1().Pods(ns).Watch(ctx, metav1.ListOptions{
		LabelSelector: runLabels.String(),
	})
	if err != nil {
		log.Fatalln("Failed to setup watch for pods", err)
	}

	pods := make([]*corev1.Pod, 0, podN)
	stats := make(map[string]*pod.Stats, podN)

	for i := 0; i < podN; i++ {
		p := podFn(ns, typ+"-"+uuid.NewString())
		p.Labels = runLabels
		pods = append(pods, p)
		stats[p.Name] = &pod.Stats{}
	}

	readyCh := make(chan struct{}, podN)
	deletedCh := make(chan struct{}, podN)
	ipCh := make(chan *corev1.Pod, podN)
	probedCh := make(chan struct{}, podN)
	go func() {
		for event := range watcher.ResultChan() {
			now := time.Now()
			switch event.Type {
			case watch.Added:
				p := event.Object.(*corev1.Pod)
				stats := stats[p.Name]
				stats.Created = now
				if p.Status.PodIP != "" && stats.HasIP.IsZero() {
					stats.HasIP = now
					ipCh <- p
				}
			case watch.Modified:
				p := event.Object.(*corev1.Pod)
				stats := stats[p.Name]

				if p.Status.PodIP != "" && stats.HasIP.IsZero() {
					stats.HasIP = now
					ipCh <- p
				}
				if pod.IsConditionTrue(p, corev1.PodScheduled) && stats.Scheduled.IsZero() {
					stats.Scheduled = now
				}
				if pod.IsConditionTrue(p, corev1.PodInitialized) && stats.Initialized.IsZero() {
					stats.Initialized = now
				}
				if pod.IsConditionTrue(p, corev1.ContainersReady) && stats.ContainersReady.IsZero() {
					stats.ContainersReady = now
				}
				if pod.IsConditionTrue(p, corev1.PodReady) && stats.Ready.IsZero() {
					stats.Ready = now
					stats.ContainersStarted = pod.LastContainerStartedTime(p)
					readyCh <- struct{}{}
				}
			case watch.Deleted:
				deletedCh <- struct{}{}
			}
		}
	}()

	if probe {
		go func() {
			for p := range ipCh {
				// TODO: Probe path needs to be adjustable per app.
				url := "http://" + p.Status.PodIP + ":8012"
				wait.PollImmediateInfinite(10*time.Millisecond, func() (bool, error) {
					resp, err := http.Get(url)
					if err != nil {
						return false, nil
					}
					defer resp.Body.Close()
					return resp.StatusCode == http.StatusOK, nil
				})
				stats[p.Name].Probed = time.Now()
				probedCh <- struct{}{}
			}
		}()
	}

	for _, p := range pods {
		if _, err := kube.CoreV1().Pods(p.Namespace).Create(ctx, p, metav1.CreateOptions{}); err != nil {
			log.Fatalln("Failed to create pod", err)
		}

		// Wait for all pods to become ready.
		if err := waitForN(ctx, readyCh, 1); err != nil {
			log.Fatalln("Failed to wait for pod becoming ready", err)
		}
		if probe {
			// And for all pods to be probed, if we're doing that.
			if err := waitForN(ctx, probedCh, 1); err != nil {
				log.Fatal("Failed to wait for pod be probed", err)
			}
		}

		if !skipDelete {
			var zero int64
			if err := kube.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{
				GracePeriodSeconds: &zero,
			}); err != nil {
				log.Fatalln("Failed to delete pod", err)
			}

			waitForN(ctx, deletedCh, 1)
		}
	}

	timeToScheduled := make([]float64, 0, len(stats))
	timeToIP := make([]float64, 0, len(stats))
	timeToProbed := make([]float64, 0, len(stats))
	timeToReady := make([]float64, 0, len(stats))
	for _, stat := range stats {
		timeToScheduled = append(timeToScheduled, float64(stat.TimeToScheduled()/time.Millisecond))
		timeToIP = append(timeToIP, float64(stat.TimeToIP()/time.Millisecond))
		timeToProbed = append(timeToProbed, float64(stat.TimeToProbed()/time.Millisecond))
		timeToReady = append(timeToReady, float64(stat.TimeToReady()/time.Millisecond))
	}

	fmt.Printf("Created %d %s pods sequentially, results are in ms:\n", podN, typ)
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "metric\tmin\tmax\tmean\tmedian\tp25\tp75\tp95\tp99")
	printStats(w, "Time to scheduled", timeToScheduled)
	printStats(w, "Time to ip", timeToIP)
	if probe {
		printStats(w, "Time to probed", timeToProbed)
	}
	printStats(w, "Time to ready", timeToReady)
	w.Flush()

	if details {
		fmt.Println()
		fmt.Println("Details:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
		fmt.Fprintln(w, "pod\tto scheduled\tto ip\tto ready")
		for name, stat := range stats {
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", name,
				stat.TimeToScheduled()/time.Millisecond,
				stat.TimeToIP()/time.Millisecond,
				stat.TimeToReady()/time.Millisecond)
		}
		w.Flush()
	}
}

func waitForN(ctx context.Context, ch chan struct{}, n int) error {
	var seen int
	for {
		select {
		case <-ch:
			seen++
			if seen == n {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func printStats(w io.Writer, label string, data []float64) {
	min, _ := statistics.Min(data)
	max, _ := statistics.Max(data)
	mean, _ := statistics.Mean(data)
	median, _ := statistics.Median(data)
	p25, _ := statistics.Percentile(data, 25)
	p75, _ := statistics.Percentile(data, 75)
	p95, _ := statistics.Percentile(data, 95)
	p99, _ := statistics.Percentile(data, 99)
	fmt.Fprintf(w, "%s\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\n", label, min, max, mean, median, p25, p75, p95, p99)
}

func prepullImages(ctx context.Context, client clientappsv1.DaemonSetInterface, podSpec corev1.PodSpec) error {
	labels := map[string]string{
		"podspeed/warmup": "true",
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "warmup",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}

	if _, err := client.Create(ctx, ds, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create DaemonSet: %w", err)
	}

	if err := wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		got, err := client.Get(ctx, ds.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to fetch DaemonSet: %w", err)
		}
		return got.Status.NumberReady > 0 && got.Status.NumberReady == got.Status.DesiredNumberScheduled, nil
	}); err != nil {
		return fmt.Errorf("DaemonSet never became ready: %w", err)
	}

	return client.Delete(ctx, ds.Name, metav1.DeleteOptions{})
}
