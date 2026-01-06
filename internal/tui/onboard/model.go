package onboard

import (
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
)

type dockerType int

const (
	dockerTypeNone dockerType = iota
	dockerTypeCompose
	dockerTypeFile
)

type onboardStep int

const (
	stepValidateRepo onboardStep = iota
	stepIntro
	stepSDKCompatibility
	stepRecordingIntro
	stepRecordingSamplingRate
	stepReplayIntro
	stepDockerSetup
	stepDockerType
	stepServiceName
	stepServicePort
	stepStartCommand
	stepStopCommand
	stepReadinessCommand
	stepReadinessTimeout
	stepReadinessInterval
	stepConfirm
	stepDone
)

type Model struct {
	// navigation
	stepIdx int
	history []int
	flow    *Flow
	inputs  []textinput.Model
	width   int
	height  int

	// Viewport for scrollable content (used in confirm step)
	viewport            viewport.Model
	viewportReady       bool
	lastViewportContent string // For preserving scroll

	// State
	ServiceName       string
	ServicePort       string
	StartCmd          string
	StopCmd           string
	ReadinessCmd      string
	ReadinessTimeout  string
	ReadinessInterval string
	SamplingRate      string

	UseDocker                bool
	DockerType               dockerType
	DockerImageName          string
	DockerAppName            string
	DockerComposeServiceName string // For docker compose override, may not be the same as ServiceName

	SDKCompatible          bool
	SDKPackagesDescription string // Dynamically fetched from SDK manifest
	ProjectType            string // Detected project type: "nodejs", "python", or ""

	Err           error
	ValidationErr error
}

// Derived config helpers (defaults identical to previous behavior).
func (m *Model) currentPortInt() int {
	port := 3000
	if p, err := strconv.Atoi(trimFirstToken(m.ServicePort)); err == nil {
		port = p
	}
	return port
}
