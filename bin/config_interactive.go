package main

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey"
	"github.com/Velocidex/yaml"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/users"
)

const (
	self_signed = "Self Signed SSL"
	autocert    = "Automatically provision certificates with Lets Encrypt"
)

var (
	deployment_type = &survey.Select{
		Message: `
Welcome to the Velociraptor configuration generator
---------------------------------------------------

I will be creating a new deployment configuration for you. I will
begin by identifying what type of deployment you need.

`,
		Options: []string{self_signed, autocert},
	}

	url_question = &survey.Input{
		Message: "What is the public DNS name of the Frontend " +
			"(e.g. www.example.com):",
		Help: "Clients will connect to the Frontend using this " +
			"public name (e.g. https://www.example.com:8000/ ).",
		Default: "localhost",
	}

	port_question = &survey.Input{
		Message: "Enter the frontend port to listen on.",
		Default: "8000",
	}

	data_store_question = &survey.Input{
		Message: "Path to the datastore directory.",
		Default: os.TempDir(),
	}

	log_question = &survey.Input{
		Message: "Path to the logs directory.",
		Default: os.TempDir(),
	}

	output_question = &survey.Input{
		Message: "Where should i write the server config file?",
		Default: "server.config.yaml",
	}

	client_output_question = &survey.Input{
		Message: "Where should i write the client config file?",
		Default: "client.config.yaml",
	}

	user_name_question = &survey.Input{
		Message: "GUI Username or email address to authorize (empty to end):",
	}
	password_question = &survey.Password{
		Message: "Password",
	}
)

func doGenerateConfigInteractive() {
	install_type := ""
	err := survey.AskOne(deployment_type, &install_type, nil)
	kingpin.FatalIfError(err, "Question")

	fmt.Println("Generating keys please wait....")
	config_obj, err := generateNewKeys()
	kingpin.FatalIfError(err, "Generating Keys")

	switch install_type {
	case self_signed:
		err = survey.AskOne(port_question, &config_obj.Frontend.BindPort, nil)
		kingpin.FatalIfError(err, "Question")

		hostname := ""
		err = survey.AskOne(url_question, &hostname, survey.Required)
		kingpin.FatalIfError(err, "Question")

		config_obj.Client.ServerUrls = append(
			config_obj.Client.ServerUrls,
			fmt.Sprintf("https://%s:%d/", hostname,
				config_obj.Frontend.BindPort))

		err = getFileStoreLocation(config_obj)
		kingpin.FatalIfError(err, "getFileStoreLocation")

		err = getLogLocation(config_obj)
		kingpin.FatalIfError(err, "getLogLocation")

	case autocert:
		// In autocert mode these are all fixed.
		config_obj.Frontend.BindPort = 443
		config_obj.GUI.BindPort = 443
		config_obj.Frontend.BindAddress = "0.0.0.0"

		hostname := ""
		err = survey.AskOne(url_question, &hostname, survey.Required)
		kingpin.FatalIfError(err, "Question")

		config_obj.Client.ServerUrls = []string{
			fmt.Sprintf("https://%s/", hostname)}

		err = getFileStoreLocation(config_obj)
		kingpin.FatalIfError(err, "getFileStoreLocation")
		err = getLogLocation(config_obj)
		kingpin.FatalIfError(err, "getLogLocation")

		config_obj.AutocertDomain = hostname
		config_obj.AutocertCertCache = config_obj.Datastore.Location

	}

	path := ""
	err = survey.AskOne(output_question, &path, survey.Required)
	kingpin.FatalIfError(err, "Question")

	res, err := yaml.Marshal(config_obj)
	kingpin.FatalIfError(err, "Yaml Marshal")

	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	kingpin.FatalIfError(err, "Open file %s", path)
	defer fd.Close()

	fd.Write(res)

	err = survey.AskOne(client_output_question, &path, survey.Required)
	kingpin.FatalIfError(err, "Question")

	client_config := getClientConfig(config_obj)
	res, err = yaml.Marshal(client_config)
	kingpin.FatalIfError(err, "Yaml Marshal")

	fd, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	kingpin.FatalIfError(err, "Open file %s", path)
	defer fd.Close()

	fd.Write(res)

	kingpin.FatalIfError(addUser(config_obj), "Add users")
}

func getFileStoreLocation(config_obj *api_proto.Config) error {
	err := survey.AskOne(data_store_question,
		&config_obj.Datastore.Location, func(val interface{}) error {
			// Check that the directory exists.
			stat, err := os.Stat(val.(string))
			if err == nil && stat.IsDir() {
				return nil
			}
			return err
		})
	if err != nil {
		return err
	}

	config_obj.Datastore.FilestoreDirectory = config_obj.Datastore.Location
	return nil
}

func getLogLocation(config_obj *api_proto.Config) error {
	err := survey.AskOne(log_question,
		&config_obj.Logging.OutputDirectory, func(val interface{}) error {
			// Check that the directory exists.
			stat, err := os.Stat(val.(string))
			if err == nil && stat.IsDir() {
				return nil
			}
			return err
		})
	if err != nil {
		return err
	}

	config_obj.Logging.SeparateLogsPerComponent = true
	return nil
}

func addUser(config_obj *api_proto.Config) error {
	for {
		username := ""
		err := survey.AskOne(user_name_question, &username, nil)
		if err != nil {
			return err
		}

		if username == "" {
			return nil
		}

		user_record, err := users.NewUserRecord(username)
		if err != nil {
			return err
		}

		if config_obj.GUI.GoogleOauthClientId != "" {
			fmt.Printf("Authentication will occur via Google - " +
				"therefore no password needs to be set.")
		} else {
			password := ""
			err := survey.AskOne(password_question, &password, survey.Required)
			if err != nil {
				return err
			}

			user_record.SetPassword(password)
		}

		err = users.SetUser(config_obj, user_record)
		if err != nil {
			return err
		}
	}
}