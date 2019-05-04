package orion

import (
	"../../blockchains/helpers"
	"../../blockchains/registrar"
	"../../db"
	"../../ssh"
	"../../testnet"
	"../../util"
	"fmt"
	"github.com/Whiteblock/mustache"
	"log"
)

var conf *util.Config

const sidecar = "orion"

func init() {
	conf = util.GetConfig()
	registrar.RegisterSideCar(sidecar, registrar.SideCar{
		Image: "gcr.io/whiteblock/orion:dev",
	})
	registrar.RegisterBuildSideCar(sidecar, build)
	registrar.RegisterAddSideCar(sidecar, add)
}

func build(tn *testnet.Adjunct) error {

	helpers.AllNodeExecConSC(tn, func(client *ssh.Client, _ *db.Server, node ssh.Node) error { //ignore err
		_, err := client.DockerExec(node, "mkdir -p /orion/data")
		return err
	})

	err := helpers.CreateConfigsSC(tn, "/orion/data/orion.conf", func(node ssh.Node) ([]byte, error) {
		return makeNodeConfig(node)
	})
	if err != nil {
		return util.LogError(err)
	}

	err = helpers.AllNodeExecConSC(tn, func(client *ssh.Client, server *db.Server, node ssh.Node) error {
		_, err := client.DockerExec(node, "bash -c 'cd /orion/data && echo \"\" | orion -g nodeKey'")
		return err
	})
	if err != nil {
		return util.LogError(err)
	}

	return helpers.AllNodeExecConSC(tn, func(client *ssh.Client, server *db.Server, node ssh.Node) error {
		return client.DockerExecdLog(node, "orion /orion/data/orion.conf")
	})
}

func add(tn *testnet.Adjunct) error {
	return nil
}

func makeNodeConfig(node ssh.Node) ([]byte, error) {

	dat, err := helpers.GetStaticBlockchainConfig(sidecar, "orion.conf.mustache")
	if err != nil {
		return nil, util.LogError(err)
	}
	data, err := mustache.Render(string(dat), util.ConvertToStringMap(map[string]interface{}{
		"nodeurl":   fmt.Sprintf("http://%s:8080/", node.GetIP()),
		"clienturl": fmt.Sprintf("http://%s:8080/", node.GetIP()),
	}))
	return []byte(data), err
}
