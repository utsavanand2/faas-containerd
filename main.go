package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	bootstrap "github.com/openfaas/faas-provider"
	"github.com/openfaas/faas-provider/types"

	"github.com/openfaas/faas/gateway/requests"
)

func main() {
	sock := os.Getenv("sock")
	client, err := containerd.New(sock)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	bootstrapHandlers := types.FaaSHandlers{
		FunctionProxy:  func(w http.ResponseWriter, r *http.Request) {},
		DeleteHandler:  deleteHandler(),
		DeployHandler:  deployHandler(),
		FunctionReader: readHandler(),
		ReplicaReader:  replicaReader(),
		ReplicaUpdater: func(w http.ResponseWriter, r *http.Request) {},
		UpdateHandler:  updateHandler(client),
		Health:         func(w http.ResponseWriter, r *http.Request) {},

		InfoHandler: func(w http.ResponseWriter, r *http.Request) {},
	}

	var port int
	port = 8082

	bootstrapConfig := types.FaaSConfig{
		ReadTimeout:     time.Second * 8,
		WriteTimeout:    time.Second * 8,
		TCPPort:         &port,
		EnableBasicAuth: false,
		EnableHealth:    false,
	}

	bootstrap.Serve(&bootstrapHandlers, &bootstrapConfig)
}

func deleteHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)

	}
}

func replicaReader() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)

	}
}

func readHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)

	}
}

func deployHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)

		defer r.Body.Close()

		body, _ := ioutil.ReadAll(r.Body)
		fmt.Println(string(body))
	}
}

func updateHandler(client *containerd.Client) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		w.WriteHeader(http.StatusOK)
		req := requests.CreateFunctionRequest{}

		defer r.Body.Close()

		body, _ := ioutil.ReadAll(r.Body)
		fmt.Println(string(body))

		json.Unmarshal(body, &req)

		go func() {
			ctx := namespaces.WithNamespace(context.Background(), "openfaas-fn")
			req.Image = "docker.io/" + req.Image

			image, err := client.Pull(ctx, req.Image, containerd.WithPullUnpack)
			if err != nil {
				log.Println(err)
				return
			}

			log.Println(image.Name())
			log.Println(image.Size(ctx))

			hook := func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
				if s.Hooks == nil {
					s.Hooks = &specs.Hooks{}
				}
				netnsPath, err := exec.LookPath("netns")
				if err != nil {
					return err
				}

				s.Hooks.Prestart = []specs.Hook{
					{
						Path: netnsPath,
						Args: []string{
							"netns",
						},
						Env: os.Environ(),
					},
				}
				return nil
			}

			id := req.Service

			container, err := client.NewContainer(
				ctx,
				id,
				containerd.WithImage(image),
				containerd.WithNewSnapshot(req.Service+"-snapshot", image),
				containerd.WithNewSpec(oci.WithImageConfig(image), hook),
			)

			if err != nil {
				log.Println(err)
				return
			}

			defer container.Delete(ctx, containerd.WithSnapshotCleanup)

			// create a task from the container
			task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
			if err != nil {
				log.Println(err)
				return
			}
			defer task.Delete(ctx)

			// make sure we wait before calling start
			exitStatusC, err := task.Wait(ctx)
			if err != nil {
				log.Println(err)
				return
			}
			log.Println(exitStatusC)

			// call start on the task to execute the redis server
			if err := task.Start(ctx); err != nil {
				log.Println(err)
				return
			}

			// sleep for a lil bit to see the logs
			time.Sleep(3 * time.Minute)

			// kill the process and get the exit status
			if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
				log.Println(err)
				return
			}

		}()

	}
}
