package internal

import (
	"crypto/tls"
	"fmt"
	"github.com/MythicMeta/Mythic_CLI/cmd/config"
	"github.com/MythicMeta/Mythic_CLI/cmd/manager"
	"github.com/streadway/amqp"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func TestMythicConnection() {
	webAddress := "127.0.0.1"
	mythicEnv := config.GetMythicEnv()
	if mythicEnv.GetString("NGINX_HOST") == "mythic_nginx" {
		if mythicEnv.GetBool("NGINX_USE_SSL") {
			webAddress = "https://127.0.0.1"
		} else {
			webAddress = "http://127.0.0.1"
		}
	} else {
		if mythicEnv.GetBool("NGINX_USE_SSL") {
			webAddress = "https://" + mythicEnv.GetString("NGINX_HOST")
		} else {
			webAddress = "http://" + mythicEnv.GetString("NGINX_HOST")
		}
	}
	maxCount := 10
	sleepTime := int64(10)
	count := make([]int, maxCount)
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	log.Printf("[*] Waiting for Mythic Server and Nginx to come online (Retry Count = %d)\n", maxCount)
	for i := range count {
		log.Printf("[*] Attempting to connect to Mythic UI at %s:%d, attempt %d/%d\n", webAddress, mythicEnv.GetInt("NGINX_PORT"), i+1, maxCount)
		resp, err := http.Get(webAddress + ":" + strconv.Itoa(mythicEnv.GetInt("NGINX_PORT")))
		if err != nil {
			log.Printf("[-] Failed to make connection to host, retrying in %ds\n", sleepTime)
			log.Printf("%v\n", err)
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 404 {
				log.Printf("[+] Successfully connected to Mythic at " + webAddress + ":" + strconv.Itoa(mythicEnv.GetInt("NGINX_PORT")) + "\n\n")
				return
			} else if resp.StatusCode == 502 || resp.StatusCode == 504 {
				log.Printf("[-] Nginx is up, but waiting for Mythic Server, retrying connection in %ds\n", sleepTime)
			} else {
				log.Printf("[-] Connection failed with HTTP Status Code %d, retrying in %ds\n", resp.StatusCode, sleepTime)
			}
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
	log.Printf("[-] Failed to make connection to Mythic Server\n")
	log.Printf("    This could be due to limited resources on the host (recommended at least 2CPU and 4GB RAM)\n")
	log.Printf("    If there is an issue with Mythic server, use 'mythic-cli logs mythic_server' to view potential errors\n")
	Status(false)
	log.Printf("[*] Fetching logs from mythic_server now:\n")
	GetLogs("mythic_server", "500")
	os.Exit(1)
}
func TestMythicRabbitmqConnection() {
	rabbitmqAddress := "127.0.0.1"
	mythicEnv := config.GetMythicEnv()
	rabbitmqPort := mythicEnv.GetString("RABBITMQ_PORT")
	if mythicEnv.GetString("RABBITMQ_HOST") != "mythic_rabbitmq" && mythicEnv.GetString("RABBITMQ_HOST") != "127.0.0.1" {
		rabbitmqAddress = mythicEnv.GetString("RABBITMQ_HOST")
	}
	if rabbitmqAddress == "127.0.0.1" && !manager.GetManager().IsServiceRunning("mythic_rabbitmq") {
		log.Printf("[-] Service mythic_rabbitmq should be running on the host, but isn't. Containers will be unable to connect.\nStart it by starting Mythic ('sudo ./mythic-cli mythic start') or manually with 'sudo ./mythic-cli mythic start mythic_rabbitmq'\n")
		return
	}
	maxCount := 10
	var err error
	count := make([]int, maxCount)
	sleepTime := int64(10)
	log.Printf("[*] Waiting for RabbitMQ to come online (Retry Count = %d)\n", maxCount)
	for i := range count {
		log.Printf("[*] Attempting to connect to RabbitMQ at %s:%s, attempt %d/%d\n", rabbitmqAddress, rabbitmqPort, i+1, maxCount)
		conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%s/mythic_vhost", mythicEnv.GetString("RABBITMQ_USER"), mythicEnv.GetString("RABBITMQ_PASSWORD"), rabbitmqAddress, rabbitmqPort))
		if err != nil {
			log.Printf("[-] Failed to connect to RabbitMQ, retrying in %ds\n", sleepTime)
			time.Sleep(10 * time.Second)
		} else {
			conn.Close()
			log.Printf("[+] Successfully connected to RabbitMQ at amqp://%s:***@%s:%s/mythic_vhost\n\n", mythicEnv.GetString("RABBITMQ_USER"), rabbitmqAddress, rabbitmqPort)
			return
		}
	}
	log.Printf("[-] Failed to make a connection to the RabbitMQ server: %v\n", err)
	if manager.GetManager().IsServiceRunning("mythic_rabbitmq") {
		log.Printf("    The mythic_rabbitmq service is running, but mythic-cli is unable to connect\n")
	} else {
		if rabbitmqAddress == "127.0.0.1" {
			log.Printf("    The mythic_rabbitmq service isn't running, but should be running locally. Did you start it?\n")
		} else {
			log.Printf("    The mythic_rabbitmq service isn't running locally, check to make sure it's running with the proper credentials\n")
		}

	}
}
func TestPorts() error {
	manager.GetManager().TestPorts()
	return nil
}

func Status(verbose bool) {
	manager.GetManager().PrintConnectionInfo()
	manager.GetManager().Status(verbose)
	installedServices, err := manager.GetManager().GetInstalled3rdPartyServicesOnDisk()
	if err != nil {
		log.Fatalf("[-] failed to get installed services: %v\n", err)
	}
	mythicEnv := config.GetMythicEnv()
	if len(installedServices) == 0 {
		log.Printf("[*] There are no services installed\n")
		log.Printf("    To install one, use \"sudo ./mythic-cli install github <url>\"\n")
		log.Printf("    Agents can be found at: https://github.com/MythicAgents\n")
		log.Printf("    C2 Profiles can be found at: https://github.com/MythicC2Profiles\n")
	}
	if mythicEnv.GetString("RABBITMQ_HOST") == "mythic_rabbitmq" && mythicEnv.GetBool("rabbitmq_bind_localhost_only") {
		log.Printf("\n[*] RabbitMQ is currently listening on localhost. If you have a remote Service, they will be unable to connect (i.e. one running on another server)")
		log.Printf("\n    Use 'sudo ./mythic-cli config set rabbitmq_bind_localhost_only false' and restart mythic ('sudo ./mythic-cli restart') to change this\n")
	}
	if mythicEnv.GetString("MYTHIC_SERVER_HOST") == "mythic_server" && mythicEnv.GetBool("mythic_server_bind_localhost_only") {
		log.Printf("\n[*] MythicServer is currently listening on localhost. If you have a remote Service, they will be unable to connect (i.e. one running on another server)")
		log.Printf("\n    Use 'sudo ./mythic-cli config set mythic_server_bind_localhost_only false' and restart mythic ('sudo ./mythic-cli restart') to change this\n")
	}
	log.Printf("[*] If you are using a remote PayloadType or C2Profile, they will need certain environment variables to properly connect to Mythic.\n")
	log.Printf("    Use 'sudo ./mythic-cli config service' for configs for these services.\n")
}
func GetLogs(containerName string, numLogs string) {
	logCount, err := strconv.Atoi(numLogs)
	if err != nil {
		log.Fatalf("[-] Bad log count: %v\n", err)
	}
	manager.GetManager().GetLogs(containerName, logCount)
}
func ListServices() {
	manager.GetManager().ListServices()
}
