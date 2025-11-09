package onboardcloud

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	tea "github.com/charmbracelet/bubbletea"
)

func Run() error {
	authenticator, err := ensureAuthenticated()
	if err != nil {
		return err
	}

	m := initialModel()

	if err := fetchUserClients(&m, authenticator); err != nil {
		slog.Debug("Failed to fetch user info", "error", err)
		return err
	}

	m.flow = createFlow()

	step := m.flow.Current(0)
	if step != nil {
		inputIdx := step.InputIndex()
		if inputIdx >= 0 && inputIdx < len(m.inputs) {
			m.inputs[inputIdx].Focus()
			m.inputs[inputIdx].Placeholder = step.Default(&m)
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	if err != nil {
		return fmt.Errorf("error running wizard: %w", err)
	}

	return nil
}

func fetchUserClients(m *Model, authenticator *auth.Authenticator) error {
	slog.Debug("Fetching user clients")

	client, authOptions, _, err := api.SetupCloud(context.Background(), false)
	if err != nil {
		return fmt.Errorf("failed to setup cloud connection: %w", err)
	}

	req := &backend.GetAuthInfoRequest{}
	resp, err := client.GetAuthInfo(context.Background(), req, authOptions)
	if err != nil {
		if strings.Contains(err.Error(), "http 401") {
			return fmt.Errorf("authentication failed - your session may have expired.\n\nPlease try running 'tusk logout' followed by 'tusk login' to refresh your credentials.\n\nIf the issue persists, please contact support at support@usetusk.ai")
		}
		return fmt.Errorf("failed to get auth info: %w", err)
	}

	m.UserId = resp.User.Id
	m.UserEmail = ""
	if resp.User.Email != nil {
		m.UserEmail = *resp.User.Email
	} else if resp.User.CodeHostingUsername != nil {
		m.UserEmail = *resp.User.CodeHostingUsername
	}
	m.IsLoggedIn = true
	m.BearerToken = authenticator.AccessToken

	m.AvailableClients = make([]ClientInfo, len(resp.Clients))
	for i, c := range resp.Clients {
		name := "Unnamed Team"
		if c.Name != nil {
			name = *c.Name
		}
		resources := make([]CodeHostingResource, len(c.CodeHostingResources))
		for j, r := range c.CodeHostingResources {
			resources[j] = CodeHostingResource{
				ID:         r.Id,
				Type:       r.Type.String(),
				ExternalID: r.ExternalId,
			}
		}
		m.AvailableClients[i] = ClientInfo{
			ID:                   c.Id,
			Name:                 name,
			CodeHostingResources: resources,
		}
	}

	if len(m.AvailableClients) == 1 {
		m.SelectedClient = &m.AvailableClients[0]
		slog.Debug("Using client", "id", m.AvailableClients[0].ID)
	} else {
		ids := make([]string, len(m.AvailableClients))
		for i, c := range m.AvailableClients {
			ids[i] = c.ID
		}
		slog.Debug("Found clients", "ids", ids)
	}

	return nil
}

func ensureAuthenticated() (*auth.Authenticator, error) {
	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authenticator: %w", err)
	}

	if err := authenticator.TryExistingAuth(context.Background()); err == nil {
		slog.Debug("Logged in", "email", authenticator.Email)
		return authenticator, nil
	}

	fmt.Print("\n🔐 Sign in to Tusk to continue.\n\n")
	fmt.Println("Press [enter] to open your browser and login...")
	_, _ = fmt.Scanln() // Wait for user to press Enter

	if err := authenticator.Login(context.Background()); err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	fmt.Printf("\n✅ Successfully logged in as %s\n\n", authenticator.Email)
	return authenticator, nil
}
