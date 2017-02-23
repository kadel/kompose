package docker

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/kubernetes-incubator/kompose/pkg/utils/archive"
)

// Build builds a Docker image from source and returns the image ID
func Build(source string, image string) error {
	ctx := context.Background()
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}

	tmpfile, err := ioutil.TempFile("/tmp", "kompose-image-build-")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name())

	// Create tarball for Docker source dir
	archive.CreateTarball(strings.Join([]string{source, ""}, "/"), tmpfile.Name())
	file, err := os.Open(tmpfile.Name())
	if err != nil {
		return err
	}

	// Build Docker image
	out, err := cli.ImageBuild(ctx, file, types.ImageBuildOptions{Tags: []string{image}})
	if err != nil {
		return err
	}
	io.Copy(os.Stdout, out.Body)
	out.Body.Close()

	return nil
}

// Push pushes a Docker image to a specided Docker registry in the image name
func Push(image string) error {
	ctx := context.Background()
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	username := os.Getenv("DOCKER_USERNAME")
	password := os.Getenv("DOCKER_PASSWORD")
	fmt.Println("Username: ", username, "Password: ", password)
	registryAuth := b64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))
	fmt.Println("Registry auth", registryAuth)

	logrus.Infof("Pushing Docker image: %s", image)
	// FIXME: base64 encoded value of registry credentials shoule be passed in
	// RegistryAuth in types.ImagePushOptions. Challenges are retrieving this
	// value automatically. It can be retrieved from ~/.docker/config.json
	// or by explicitly asking for username/password from the user
	out, err := cli.ImagePush(ctx, image, types.ImagePushOptions{RegistryAuth: registryAuth})
	if err != nil {
		return err
	}
	io.Copy(os.Stdout, out)
	out.Close()

	return nil
}
