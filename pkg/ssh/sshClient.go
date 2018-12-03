package ssh

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SingleExecute - This will execute a command on a single host
func SingleExecute(cmd string, host HostSSHConfig, to int) CommandResult {
	var configs []HostSSHConfig
	configs = append(configs, host)
	result := ParalellExecute(cmd, configs, to)
	return result[0]
}

//ParalellExecute - This will execute the same command in paralell across multiple hosts
func ParalellExecute(cmd string, hosts []HostSSHConfig, to int) []CommandResult {
	var cmdResults []CommandResult
	// Run parallel ssh session (max 10)
	results := make(chan CommandResult, 10)
	timeout := time.After(time.Duration(to) * time.Second)

	// Execute command on hosts
	for _, host := range hosts {
		go func(host HostSSHConfig) {
			res := new(CommandResult)
			res.Host = host.Host

			if text, err := host.ExecuteCmd(cmd); err != nil {
				res.Error = err
			} else {
				res.Result = text
			}
			results <- *res
		}(host)
	}

	for i := 0; i < len(hosts); i++ {
		select {
		case res := <-results:
			// Append the results of a succesfull command
			cmdResults = append(cmdResults, res)
		case <-timeout:
			// In the event that a command times out then append the details
			failedCommand := CommandResult{
				Host:   hosts[i].Host,
				Error:  fmt.Errorf("Command Timed out"),
				Result: "",
			}
			cmdResults = append(cmdResults, failedCommand)

		}
	}
	return cmdResults
}

// StartConnection -
func (c *HostSSHConfig) StartConnection() (*ssh.Client, error) {
	var err error

	host := c.Host
	if !strings.ContainsAny(c.Host, ":") {
		host = host + ":22"
	}
	//log.Printf("%v", c)
	c.Connection, err = ssh.Dial("tcp", host, c.ClientConfig)
	if err != nil {
		return nil, err
	}
	return c.Connection, nil
}

// StopConnection -
func (c *HostSSHConfig) StopConnection() error {
	if c.Connection != nil {
		return c.Connection.Close()
	}
	return fmt.Errorf("Connection not established")
}

// StartSession -
func (c *HostSSHConfig) StartSession() (*ssh.Session, error) {
	var err error
	c.Connection, err = c.StartConnection()
	if err != nil {
		return nil, err
	}
	c.Session, err = c.Connection.NewSession()
	if err != nil {
		return nil, err
	}
	return c.Session, err
}

// StopSession -
func (c *HostSSHConfig) StopSession() {
	if c.Session != nil {
		c.Session.Close()
	}
}

// ExecuteCmd -
func (c *HostSSHConfig) ExecuteCmd(cmd string) (string, error) {
	if c.Session == nil {
		if _, err := c.StartSession(); err != nil {
			return "", err
		}
	}

	var stdoutBuf bytes.Buffer
	c.Session.Stdout = &stdoutBuf
	c.Session.Run(cmd)

	return stdoutBuf.String(), nil
}

// DownloadFile -
func (c HostSSHConfig) DownloadFile(source, destination string) error {
	var err error
	c.Connection, err = c.StartConnection()
	if err != nil {
		return err
	}

	// New SFTP client
	sftp, err := sftp.NewClient(c.Connection)
	if err != nil {
		return err
	}
	defer sftp.Close()

	// Open remote source
	sftpSource, err := sftp.Open(source)
	if err != nil {
		return err
	}
	defer sftpSource.Close()

	// Open local destination
	localDestination, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer localDestination.Close()

	//
	_, err = sftpSource.WriteTo(localDestination)
	if err != nil {
		return err
	}

	// An error here isn't cause for alarm, any new transaction should create a new connection
	_ = c.StopConnection()

	return nil
}

// UploadFile -
func (c HostSSHConfig) UploadFile(source, destination string) error {
	var err error
	c.Connection, err = c.StartConnection()
	if err != nil {
		return err
	}
	// New SFTP client
	sftp, err := sftp.NewClient(c.Connection)
	if err != nil {
		return err
	}
	defer sftp.Close()

	// Open remote source
	sftpDestination, err := sftp.Create(destination)
	if err != nil {
		return err
	}
	defer sftpDestination.Close()

	// Open local destination
	localSource, err := os.Open(source)
	if err != nil {
		return err
	}
	defer localSource.Close()

	// copy source file to destination file
	_, err = io.Copy(sftpDestination, localSource)
	if err != nil {
		return err
	}

	// An error here isn't cause for alarm, any new transaction should create a new connection
	_ = c.StopConnection()

	return nil
}

// To string
func (c HostSSHConfig) String() string {
	return c.User + "@" + c.Host
}