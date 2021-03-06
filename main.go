/**
 * Copyright 2019 Whiteblock Inc. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

package main

import (
	"os"

	"github.com/whiteblock/genesis/pkg/config"
	"github.com/whiteblock/genesis/pkg/controller"
	"github.com/whiteblock/genesis/pkg/file"
	"github.com/whiteblock/genesis/pkg/handler"
	handAux "github.com/whiteblock/genesis/pkg/handler/auxillary"
	"github.com/whiteblock/genesis/pkg/repository"
	"github.com/whiteblock/genesis/pkg/service"
	"github.com/whiteblock/genesis/pkg/usecase"

	"github.com/gorilla/mux"
	queue "github.com/whiteblock/amqp"
)

func getRestServer() (controller.RestController, error) {
	conf, err := config.NewConfig()
	if err != nil {
		return nil, err
	}
	config.SanityCheck(conf)

	return controller.NewRestController(
		conf.GetRestConfig(),
		handler.NewRestHandler(
			handAux.NewExecutor(
				conf.Execution,
				usecase.NewDockerUseCase(
					service.NewDockerService(
						repository.NewDockerRepository(conf.GetLogger()),
						conf.Docker,
						file.NewRemoteSources(
							conf,
							conf.GetLogger()),
						conf.GetLogger()),
					conf.GetLogger()),
				conf.GetLogger()),
			conf.GetLogger()),
		mux.NewRouter(),
		conf.GetLogger()), nil
}

func getCommandController() (controller.CommandController, error) {
	conf, err := config.NewConfig()
	if err != nil {
		return nil, err
	}
	config.SanityCheck(conf)
	if conf.Execution.DebugMode {
		conf.GetLogger().Warn("Debug mode is enabled!")
	}

	complConf, err := conf.CompletionAMQP()
	if err != nil {
		return nil, err
	}

	cmdConf, err := conf.CommandAMQP()
	if err != nil {
		return nil, err
	}

	errConf, err := conf.ErrorsAMQP()
	if err != nil {
		return nil, err
	}

	statusConf, err := conf.StatusAMQP()
	if err != nil {
		return nil, err
	}

	queue.AssertUniqueQueues(conf.GetLogger(), complConf, cmdConf, errConf, statusConf)

	cmdConn, err := queue.OpenAMQPConnection(cmdConf.Endpoint)
	if err != nil {
		return nil, err
	}

	complConn, err := queue.OpenAMQPConnection(complConf.Endpoint)
	if err != nil {
		return nil, err
	}

	errConn, err := queue.OpenAMQPConnection(errConf.Endpoint)
	if err != nil {
		return nil, err
	}

	statusConn, err := queue.OpenAMQPConnection(statusConf.Endpoint)
	if err != nil {
		return nil, err
	}

	return controller.NewCommandController(
		conf,
		queue.NewAMQPService(cmdConf, queue.NewAMQPRepository(cmdConn), conf.GetLogger()),
		queue.NewAMQPService(errConf, queue.NewAMQPRepository(errConn), conf.GetLogger()),
		queue.NewAMQPService(complConf, queue.NewAMQPRepository(complConn), conf.GetLogger()),
		queue.NewAMQPService(statusConf, queue.NewAMQPRepository(statusConn), conf.GetLogger()),
		handler.NewDeliveryHandler(
			handAux.NewExecutor(
				conf.Execution,
				usecase.NewDockerUseCase(
					service.NewDockerService(
						repository.NewDockerRepository(conf.GetLogger()),
						conf.Docker,
						file.NewRemoteSources(
							conf,
							conf.GetLogger()),
						conf.GetLogger()),
					conf.GetLogger()),
				conf.GetLogger()),
			conf,
			conf.MaxMessageRetries,
			conf.GetLogger()),
		conf.GetLogger()), nil
}

func main() {

	if len(os.Args) == 2 && os.Args[1] == "test" { //Run some basic docker functionality tests
		dockerTest(false)
		os.Exit(0)
	}

	if len(os.Args) == 2 && os.Args[1] == "clean" { //Clean some basic docker functionality tests
		dockerTest(true)
		os.Exit(0)
	}

	restServer, err := getRestServer()
	if err != nil {
		panic(err)
	}

	conf, err := config.NewConfig()
	if err != nil {
		panic(err)
	}

	if !conf.LocalMode {
		cmdCntl, err := getCommandController()
		if err != nil {
			panic(err)
		}
		go cmdCntl.Start()
	}

	conf.GetLogger().Info("starting the rest server")
	restServer.Start()
}
