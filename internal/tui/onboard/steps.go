package onboard

import "strconv"

func stepsList() []Step {
	return []Step{
		ValidateRepoStep{},
		IntroStep{},
		SDKCompatibilityStep{},
		RecordingIntroStep{},
		// RecordingSamplingRateStep{},
		ReplayIntroStep{},
		DockerSetupStep{},
		DockerTypeStep{},
		ServiceNameStep{},
		ServicePortStep{},
		StartCommandStep{},
		StopCommandStep{},
		ReadinessCommandStep{},
		// ReadinessTimeoutStep{},
		// ReadinessIntervalStep{},
		ConfirmStep{},
		DoneStep{},
	}
}

type ValidateRepoStep struct{ BaseStep }

func (ValidateRepoStep) ID() onboardStep           { return stepValidateRepo }
func (ValidateRepoStep) InputIndex() int           { return -1 }
func (ValidateRepoStep) Question(*Model) string    { return "" }
func (ValidateRepoStep) Description(*Model) string { return "" }
func (ValidateRepoStep) Default(*Model) string     { return "" }
func (ValidateRepoStep) Help(*Model) string        { return "enter/esc: quit" }
func (ValidateRepoStep) ShouldSkip(*Model) bool    { return false }

type IntroStep struct{ BaseStep }

func (IntroStep) ID() onboardStep       { return stepIntro }
func (IntroStep) Default(*Model) string { return "" }
func (IntroStep) InputIndex() int       { return -1 }
func (IntroStep) Question(*Model) string {
	return "This wizard will help you configure Tusk Drift for your service (the current directory)."
}

func (IntroStep) Description(*Model) string {
	return "A config file, .tusk/config.yaml, will be created.\n\nPress [enter] to continue."
}
func (IntroStep) Help(*Model) string { return "enter: continue • esc: quit" }

type SDKCompatibilityStep struct{ BaseStep }

func (SDKCompatibilityStep) ID() onboardStep       { return stepSDKCompatibility }
func (SDKCompatibilityStep) Default(*Model) string { return "" }
func (SDKCompatibilityStep) InputIndex() int       { return -1 }
func (SDKCompatibilityStep) Question(*Model) string {
	return "Does your service use only packages from the list below for outbound requests?"
}

func (SDKCompatibilityStep) Description(*Model) string {
	// NOTE: make sure to use 2 spaces for indentation, NO tabs
	return `Tusk Drift Node SDK currently supports:
  • HTTP/HTTPS: All versions (Node.js built-in)
  • GRPC: @grpc/grpc-js@1.x (Outbound requests only)
  • PG: pg@8.x, pg-pool@2.x-3.x
  • Firestore: @google-cloud/firestore@7.x
  • Postgres: postgres@3.x
  • MySQL: mysql2@3.x, mysql@2.x
  • IORedis: ioredis@4.x-5.x
  • Upstash Redis: @upstash/redis@1.x
  • GraphQL: graphql@15.x-16.x
  • JSON Web Tokens: jsonwebtoken@5.x-9.x
  • JWKS RSA: jwks-rsa@1.x-3.x

Some dependencies may use one or more of these packages under the hood (e.g., your ORM may use PG).

If your service uses other packages, submit an issue at https://github.com/Use-Tusk/drift-node-sdk/issues.

Are the above packages compatible with your service? (y/n)`
}
func (SDKCompatibilityStep) Help(*Model) string { return "y: yes • n: no • esc: quit" }
func (SDKCompatibilityStep) Clear(m *Model)     { m.SDKCompatible = false }

type RecordingIntroStep struct{ BaseStep }

func (RecordingIntroStep) ID() onboardStep        { return stepRecordingIntro }
func (RecordingIntroStep) Default(*Model) string  { return "" }
func (RecordingIntroStep) InputIndex() int        { return -1 }
func (RecordingIntroStep) Question(*Model) string { return "Understanding Tusk Drift: Recording Phase" }
func (RecordingIntroStep) Description(*Model) string {
	return `Tusk Drift works in two phases:

1. RECORDING Phase (what you'll do first):
   • Run your service in production or staging
   • The Tusk SDK automatically captures traces of:
     - HTTP requests/responses
     - Database queries
     - Redis operations
     - And other external calls
   • These traces are saved locally and can be uploaded to Tusk Cloud

2. REPLAY Phase (what you'll use for testing):
   • Run your tests locally using the captured traces
   • The Tusk CLI replays mocked responses based on recorded traces
   • Tests run faster and don't need external dependencies

Press [enter] to configure recording.`
}
func (RecordingIntroStep) Help(*Model) string { return "enter: continue • esc: quit" }

type RecordingSamplingRateStep struct{ BaseStep }

func (RecordingSamplingRateStep) ID() onboardStep { return stepRecordingSamplingRate }
func (RecordingSamplingRateStep) InputIndex() int { return 0 }
func (RecordingSamplingRateStep) Question(*Model) string {
	return "What sampling rate should be used for recording traces?"
}

func (RecordingSamplingRateStep) Description(*Model) string {
	return `Sampling rate controls what percentage of requests are recorded.
  • 1.0 = record 100% of requests (recommended for development/staging)
  • 0.1 = record 10% of requests (recommended for production)

If you are setting up Tusk Drift locally for the first time, set this value to 1.0.
You can change this in .tusk/config.yaml later.`
}
func (RecordingSamplingRateStep) Default(*Model) string { return "1.0" }
func (RecordingSamplingRateStep) Validate(_ *Model, v string) error {
	_, err := parseFloatInRange(v, 0.0, 1.0, "Invalid sampling rate: must be between 0.0 and 1.0")
	return err
}
func (RecordingSamplingRateStep) Apply(m *Model, v string) { m.SamplingRate = v }
func (RecordingSamplingRateStep) Clear(m *Model)           { m.SamplingRate = "" }

type ReplayIntroStep struct{ BaseStep }

func (ReplayIntroStep) ID() onboardStep        { return stepReplayIntro }
func (ReplayIntroStep) Default(*Model) string  { return "" }
func (ReplayIntroStep) InputIndex() int        { return -1 }
func (ReplayIntroStep) Question(*Model) string { return "Understanding Tusk Drift: Replay Phase" }
func (ReplayIntroStep) Description(*Model) string {
	return `Now let's configure the REPLAY phase.

When running tests with recorded traces:
  • Your service starts with the Tusk SDK in REPLAY mode
  • External calls are intercepted and mocked using recorded data
  • Tests run against these mocks instead of real services

Press [enter] to continue.`
}
func (ReplayIntroStep) Help(*Model) string { return "enter: continue • esc: quit" }

type DockerSetupStep struct{ BaseStep }

func (DockerSetupStep) ID() onboardStep       { return stepDockerSetup }
func (DockerSetupStep) Default(*Model) string { return "" }
func (DockerSetupStep) InputIndex() int       { return -1 }
func (DockerSetupStep) Question(*Model) string {
	return "Will you be running your service with Docker or Docker Compose? (y/n)"
}

func (DockerSetupStep) Description(*Model) string {
	return "This will configure the CLI to use TCP communication instead of Unix sockets."
}
func (DockerSetupStep) Help(*Model) string { return "y: yes • n: no • esc: quit" }
func (DockerSetupStep) Clear(m *Model)     { m.UseDocker = false; m.DockerType = dockerTypeNone }

type DockerTypeStep struct{ BaseStep }

func (DockerTypeStep) ID() onboardStep        { return stepDockerType }
func (DockerTypeStep) Default(*Model) string  { return "" }
func (DockerTypeStep) InputIndex() int        { return -1 }
func (DockerTypeStep) Question(*Model) string { return "Are you using Docker Compose or Dockerfile?" }
func (DockerTypeStep) Description(*Model) string {
	return "Press 'c' for Docker Compose or 'd' for Dockerfile"
}
func (DockerTypeStep) Help(*Model) string       { return "c: Docker Compose • d: Dockerfile • esc: quit" }
func (DockerTypeStep) ShouldSkip(m *Model) bool { return !m.UseDocker }

type ServiceNameStep struct{ BaseStep }

func (ServiceNameStep) ID() onboardStep           { return stepServiceName }
func (ServiceNameStep) InputIndex() int           { return 1 }
func (ServiceNameStep) Question(*Model) string    { return "What's the name of your service?" }
func (ServiceNameStep) Description(*Model) string { return "e.g., \"acme-backend\"" }
func (s ServiceNameStep) Default(m *Model) string { return m.serviceNameDefault() }
func (ServiceNameStep) Apply(m *Model, v string)  { m.ServiceName = v }
func (ServiceNameStep) Clear(m *Model)            { m.ServiceName = "" }

type ServicePortStep struct{ BaseStep }

func (ServicePortStep) ID() onboardStep           { return stepServicePort }
func (ServicePortStep) InputIndex() int           { return 2 }
func (ServicePortStep) Question(*Model) string    { return "What port does your service run on?" }
func (ServicePortStep) Description(*Model) string { return "" }
func (s ServicePortStep) Default(m *Model) string { return m.servicePortDefault() }
func (ServicePortStep) Validate(_ *Model, v string) error {
	_, err := strconv.Atoi(trimFirstToken(v))
	if err != nil {
		return errInvalidPort()
	}
	return nil
}
func (ServicePortStep) Apply(m *Model, v string) { m.ServicePort = trimFirstToken(v) }
func (ServicePortStep) Clear(m *Model)           { m.ServicePort = "" }

type StartCommandStep struct{ BaseStep }

func (StartCommandStep) ID() onboardStep               { return stepStartCommand }
func (StartCommandStep) InputIndex() int               { return 3 }
func (s StartCommandStep) Question(m *Model) string    { return m.startCommandQuestion() }
func (s StartCommandStep) Description(m *Model) string { return m.startCommandDescription() }
func (s StartCommandStep) Default(m *Model) string     { return m.startCommandDefault() }
func (StartCommandStep) Apply(m *Model, v string)      { m.StartCmd = v }
func (StartCommandStep) Clear(m *Model)                { m.StartCmd = "" }

type StopCommandStep struct{ BaseStep }

func (StopCommandStep) ID() onboardStep           { return stepStopCommand }
func (StopCommandStep) InputIndex() int           { return 4 }
func (StopCommandStep) Question(*Model) string    { return "How do you stop your service?" }
func (StopCommandStep) Description(*Model) string { return "" }
func (s StopCommandStep) Default(m *Model) string { return m.stopCommandDefault() }
func (StopCommandStep) Apply(m *Model, v string)  { m.StopCmd = v }
func (StopCommandStep) ShouldSkip(m *Model) bool  { return !m.UseDocker }
func (StopCommandStep) Clear(m *Model)            { m.StopCmd = "" }

type ReadinessCommandStep struct{ BaseStep }

func (ReadinessCommandStep) ID() onboardStep { return stepReadinessCommand }
func (ReadinessCommandStep) InputIndex() int { return 5 }
func (ReadinessCommandStep) Question(*Model) string {
	return "How should we check if your service is ready?"
}

func (ReadinessCommandStep) Description(*Model) string {
	return "Command to verify your service is ready"
}
func (s ReadinessCommandStep) Default(m *Model) string { return m.readinessCommandDefault() }
func (ReadinessCommandStep) Apply(m *Model, v string)  { m.ReadinessCmd = v }
func (ReadinessCommandStep) Clear(m *Model)            { m.ReadinessCmd = "" }

type ReadinessTimeoutStep struct{ BaseStep }

func (ReadinessTimeoutStep) ID() onboardStep { return stepReadinessTimeout }
func (ReadinessTimeoutStep) InputIndex() int { return 6 }
func (ReadinessTimeoutStep) Question(*Model) string {
	return "Maximum time to wait for the service to be ready upon startup:"
}
func (ReadinessTimeoutStep) Description(*Model) string { return "e.g., 30s, 1m" }
func (ReadinessTimeoutStep) Default(*Model) string     { return "30s" }
func (ReadinessTimeoutStep) Apply(m *Model, v string)  { m.ReadinessTimeout = v }
func (ReadinessTimeoutStep) Clear(m *Model)            { m.ReadinessTimeout = "" }

type ReadinessIntervalStep struct{ BaseStep }

func (ReadinessIntervalStep) ID() onboardStep { return stepReadinessInterval }
func (ReadinessIntervalStep) InputIndex() int { return 7 }
func (ReadinessIntervalStep) Question(*Model) string {
	return "How often to check readiness upon startup:"
}
func (ReadinessIntervalStep) Description(*Model) string { return "e.g., 1s, 500ms" }
func (ReadinessIntervalStep) Default(*Model) string     { return "1s" }
func (ReadinessIntervalStep) Apply(m *Model, v string)  { m.ReadinessInterval = v }
func (ReadinessIntervalStep) Clear(m *Model)            { m.ReadinessInterval = "" }

type ConfirmStep struct{ BaseStep }

func (ConfirmStep) ID() onboardStep           { return stepConfirm }
func (ConfirmStep) Default(*Model) string     { return "" }
func (ConfirmStep) InputIndex() int           { return -1 }
func (ConfirmStep) Question(*Model) string    { return "" }
func (ConfirmStep) Description(*Model) string { return "" }
func (ConfirmStep) Help(*Model) string        { return "y: yes • n: no • esc: quit" }

type DoneStep struct{ BaseStep }

func (DoneStep) ID() onboardStep           { return stepDone }
func (DoneStep) Default(*Model) string     { return "" }
func (DoneStep) InputIndex() int           { return -1 }
func (DoneStep) Question(*Model) string    { return "" }
func (DoneStep) Description(*Model) string { return "" }
func (DoneStep) Help(*Model) string        { return "enter/esc: quit" }
