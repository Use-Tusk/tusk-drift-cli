package onboardcloud

import (
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
)

type onboardStep int

const (
	stepValidateConfig onboardStep = iota
	stepIntro
	stepLogin
	stepVerifyGitRepo
	stepSelectClient
	stepGithubAuth
	stepVerifyRepoAccess
	stepCreateObservableService
	stepCreateApiKey
	stepRecordingConfig
	stepCIWorkflow
	stepReview
	stepDone
)

type Model struct {
	// Navigation
	stepIdx  int
	history  []int
	flow     *Flow
	inputs   []textinput.Model
	width    int
	height   int
	progress progress.Model

	// State - Authentication
	IsLoggedIn       bool
	BearerToken      string
	UserId           string
	UserEmail        string
	HasApiKey        bool
	SelectedClient   *ClientInfo
	AvailableClients []ClientInfo

	// State - Git & Repo
	GitRepoOwner            string
	GitRepoName             string
	CodeHostingResourceType CodeHostingResourceType
	NeedsCodeHostingAuth    bool
	RepoAccessVerified      bool
	RepoID                  int64

	// State - Observable Service
	ServiceID      string
	ServiceCreated bool

	// State - API Key
	ApiKeyName         string
	ApiKey             string
	ApiKeyID           string
	CreateApiKeyChoice bool

	// State - Recording Config
	SamplingRate          string
	ExportSpans           bool
	EnableEnvVarRecording bool
	RecordingConfigTable  *RecordingConfigTable

	// State - CI Workflow
	CreateWorkflowFile bool
	WorkflowCreated    bool

	// Errors
	Err           error
	ValidationErr error
}

type CodeHostingResourceType int

const (
	CodeHostingResourceTypeGitHub CodeHostingResourceType = iota
	CodeHostingResourceTypeGitLab
)

type ClientInfo struct {
	ID                   string
	Name                 string
	CodeHostingResources []CodeHostingResource
}

type CodeHostingResource struct {
	ID         int64
	Type       string // "github" or "gitlab"
	ExternalID string
}

func initialModel() Model {
	inputs := make([]textinput.Model, 3)
	for i := range inputs {
		inputs[i] = textinput.New()
	}

	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 60 // Will be adjusted based on terminal width

	return Model{
		stepIdx:  0,
		history:  []int{},
		inputs:   inputs,
		progress: prog,
	}
}
