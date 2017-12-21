package triremecli

import (
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/aporeto-inc/trireme-example/config"
	"github.com/aporeto-inc/trireme-example/extractors"
	"github.com/aporeto-inc/trireme-example/policyexample"

	trireme "github.com/aporeto-inc/trireme-lib"
	"github.com/aporeto-inc/trireme-lib/cmd/systemdutil"
	"github.com/aporeto-inc/trireme-lib/enforcer/utils/secrets"
)

// KillContainerOnError defines if the Container is getting killed if the policy Application resulted in an error
const KillContainerOnError = true

// ProcessArgs handles all commands options for trireme
func ProcessArgs(config config.Configuration) (err error) {

	if config.Enforce {
		return ProcessEnforce(config)
	}

	if config.Run || arguments["<cgroup>"] != nil {
		// Execute a command or process a cgroup cleanup and exit
		return processRun(config)
	}

	// Trireme Daemon Commands
	return processDaemon(config)
}

func processEnforce(config config.Configuration) (err error) {
	// Run enforcer and exit
	if err := trireme.LaunchRemoteEnforcer(processor); err != nil {
		zap.L().Fatal("Unable to start enforcer", zap.Error(err))
	}
	return nil
}

func processRun(config config.Configuration) (err error) {
	return systemdutil.ExecuteCommandFromArguments(arguments)
}

func processDaemon(config config.Configuration) (err error) {

	triremeOptions := []trireme.Option{}

	// Setting up Secret Auth type based on user config.
	var triremesecret secrets.Secrets
	if config.AuthType == "PSK" {
		zap.L().Info("Initializing Trireme with PSK Auth. Should NOT be used in production")

		triremesecret = secrets.NewPSKSecrets([]byte(config.PSK))

	} else if config.AuthType == "PKI" {
		zap.L().Info("Initializing Trireme with PKI Auth")

		triremesecret, err = utils.LoadCompactPKI(config.KeyPath, config.CertPath, config.CaCertPath, config.CaKeyPath)
		if err != nil {
			zap.L().Fatal("error creating PKI Secret for Trireme", zap.Error(err))
		}
	} else {
		zap.L().Fatal("No Authentication option given")
	}
	triremeOptions = append(triremeOptions, trireme.OptionSecret(triremesecret))

	// Setting up extractor and monitor
	monitorOptions := []trireme.MonitorOption{}

	if config.Docker {

		if config.Swarm {
			dockerOptions = append(dockerOptions, trireme.SubOptionMonitorDockerExtractor(extractors.SwarmExtractor))
		}

		monitorOptions = append(monitorOptions, trireme.OptionMonitorDocker(dockerOptions))
	}

	if config.LinuxProcesses {
		monitorOptions = append(monitorOptions, trireme.OptionMonitorLinuxProcess())
	}

	triremeOptions = append(triremeOptions, trireme.OptopmMonitors(monitorOptions))

	// Setting up PolicyResolver
	policyEngine := policyexample.NewCustomPolicyResolver(config.ParsedTriremeNetworks, config.policyFile)
	triremeOptions = append(triremeOptions, trireme.OptionPolicyResolver(policyEngine))

	t := trireme.New(triremeNodeName, triremeOptions...)
	if t == nil {
		zap.L().Fatal("Unable to initialize trireme")
	}

	// Start all the go routines.
	t.Start()
	zap.L().Debug("Trireme started")

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	zap.L().Info("Everything started. Waiting for Stop signal")
	// Waiting for a Sig
	<-c

	zap.L().Debug("Stop signal received")
	t.Stop()
	zap.L().Debug("Trireme stopped")
	zap.L().Info("Everything stopped. Bye Trireme-Example!")

	return nil
}
