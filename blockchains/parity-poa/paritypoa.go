package paritypoa

import (
	db "../../db"
	state "../../state"
	util "../../util"
	helpers "../helpers"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

var conf *util.Config

func init() {
	conf = util.GetConfig()
}

/*
Build builds out a fresh new ethereum test network
*/
func Build(details *db.DeploymentDetails, servers []db.Server, clients []*util.SshClient,
	buildState *state.BuildState) ([]string, error) {

	mux := sync.Mutex{}
	pconf, err := NewConf(details.Params)
	fmt.Printf("%#v\n", *pconf)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	buildState.SetBuildSteps(9 + (7 * details.Nodes))
	//Make the data directories
	err = helpers.AllNodeExecCon(servers, buildState, func(serverNum int, localNodeNum int, absoluteNodeNum int) error {
		_, err := clients[serverNum].DockerExec(localNodeNum, "mkdir -p /parity")
		return err
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	buildState.IncrementBuildProgress()

	/**Create the Password file**/
	{
		var data string
		for i := 1; i <= details.Nodes; i++ {
			data += "second\n"
		}
		err = buildState.Write("passwd", data)
		if err != nil {
			log.Println(err)
			return nil, err
		}
	}
	buildState.IncrementBuildProgress()
	/**Copy over the password file**/
	err = helpers.CopyToAllNodes(servers, clients, buildState, "passwd", "/parity/")
	if err != nil {
		log.Println(err)
		return nil, err
	}
	buildState.IncrementBuildProgress()

	/**Create the wallets**/
	wallets := make([]string, details.Nodes)
	rawWallets := make([]string, details.Nodes)
	err = helpers.AllNodeExecCon(servers, buildState, func(serverNum int, localNodeNum int, absoluteNodeNum int) error {
		res, err := clients[serverNum].DockerExec(localNodeNum, "parity --base-path=/parity/ --password=/parity/passwd account new")
		if err != nil {
			log.Println(err)
			return err
		}

		if len(res) == 0 {
			return fmt.Errorf("account new returned an empty response")
		}

		mux.Lock()
		wallets[absoluteNodeNum] = res[:len(res)-1]
		mux.Unlock()

		res, err = clients[serverNum].DockerExec(localNodeNum, "bash -c 'cat /parity/keys/ethereum/*'")
		if err != nil {
			log.Println(err)
			return err
		}
		buildState.IncrementBuildProgress()

		mux.Lock()
		rawWallets[absoluteNodeNum] = strings.Replace(res, "\"", "\\\"", -1)
		mux.Unlock()
		return nil
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}

	buildState.IncrementBuildProgress()

	//Create the chain spec files
	spec, err := BuildSpec(pconf, details.Files, wallets)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = buildState.Write("spec.json", spec)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	//create config file
	configToml, err := BuildConfig(pconf, details.Files, wallets, "/parity/passwd")
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = buildState.Write("config.toml", configToml)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	//Copy over the config file, spec file, and the accounts
	err = helpers.CopyToAllNodes(servers, clients, buildState,
		"config.toml", "/parity/",
		"spec.json", "/parity/")
	if err != nil {
		log.Println(err)
		return nil, err
	}
	err = helpers.AllNodeExecCon(servers, buildState, func(serverNum int, localNodeNum int, absoluteNodeNum int) error {
		for i, rawWallet := range rawWallets {

			_, err = clients[serverNum].DockerExec(localNodeNum, fmt.Sprintf("bash -c 'echo \"%s\">/parity/account%d'", rawWallet, i))
			if err != nil {
				log.Println(err)
				return err
			}
			//buildState.Defer(func(){clients[serverNum].DockerExec(localNodeNum, fmt.Sprintf("rm /parity/account%d", i))})

			_, err = clients[serverNum].DockerExec(localNodeNum,
				fmt.Sprintf("parity --base-path=/parity/ --chain /parity/spec.json --password=/parity/passwd account import /parity/account%d", i))
			if err != nil {
				log.Println(err)
				return err
			}
		}
		buildState.IncrementBuildProgress()
		return nil
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}

	//util.Write("tmp/config.toml",configToml)
	err = helpers.AllNodeExecCon(servers, buildState, func(serverNum int, localNodeNum int, absoluteNodeNum int) error {
		defer buildState.IncrementBuildProgress()
		return clients[serverNum].DockerExecdLog(localNodeNum,
			fmt.Sprintf(`parity --author=%s -c /parity-poa/config.toml --chain=/parity/spec.json`, wallets[absoluteNodeNum]))
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	//Start peering via curl
	time.Sleep(time.Duration(5 * time.Second))
	//Get the enode addresses
	enodes := make([]string, details.Nodes)
	err = helpers.AllNodeExecCon(servers, buildState, func(serverNum int, localNodeNum int, absoluteNodeNum int) error {
		enode := ""
		for len(enode) == 0 {
			ip := servers[serverNum].Ips[localNodeNum]
			res, err := clients[serverNum].KeepTryRun(
				fmt.Sprintf(
					`curl -sS -X POST http://%s:8545 -H "Content-Type: application/json" `+
						` -d '{ "method": "parity_enode", "params": [], "id": 1, "jsonrpc": "2.0" }'`,
					ip))

			if err != nil {
				log.Println(err)
				return err
			}
			var result map[string]interface{}

			err = json.Unmarshal([]byte(res), &result)
			if err != nil {
				log.Println(err)
				return err
			}
			fmt.Println(result)

			err = util.GetJSONString(result, "result", &enode)
			if err != nil {
				log.Println(err)
				return err
			}
		}
		buildState.IncrementBuildProgress()
		mux.Lock()
		enodes[absoluteNodeNum] = enode
		mux.Unlock()
		return nil
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = helpers.AllNodeExecCon(servers, buildState, func(serverNum int, localNodeNum int, absoluteNodeNum int) error {
		ip := servers[serverNum].Ips[localNodeNum]
		for i, enode := range enodes {
			if i == absoluteNodeNum {
				continue
			}
			_, err := clients[serverNum].KeepTryRun(
				fmt.Sprintf(
					`curl -sS -X POST http://%s:8545 -H "Content-Type: application/json"  -d `+
						`'{ "method": "parity_addReservedPeer", "params": ["%s"], "id": 1, "jsonrpc": "2.0" }'`,
					ip, enode))
			buildState.IncrementBuildProgress()
			if err != nil {
				log.Println(err)
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}

	buildState.IncrementBuildProgress()

	return nil, err
}

/***************************************************************************************************************************/

func Add(details db.DeploymentDetails, servers []db.Server, clients []*util.SshClient,
	newNodes map[int][]string, buildState *state.BuildState) ([]string, error) {
	return nil, nil
}
