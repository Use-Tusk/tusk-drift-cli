# Onboarding Wizard

This directory contains the implementation of the `tusk init` onboarding wizard, a terminal UI (TUI) that guides users through configuring Tusk Drift for their service.

## Architecture Overview

The wizard is built using [Bubble Tea](https://github.com/charmbracelet/bubbletea).

### Key Components

```text
onboard/
├── run.go              # Entry point - initializes and runs the wizard
├── model.go            # Model - application state
├── flow.go             # Step interface and flow control
├── steps.go            # Step implementations (all wizard steps)
├── update.go           # Update - handles user input and state transitions
├── view.go             # View - renders the UI
├── helpers.go          # Helper functions (defaults, validation, state clearing)
├── config.go           # Config types and YAML generation
├── yaml.go             # YAML formatting utilities
├── docker.go           # Docker detection helpers
├── detect_project.go   # Project type detection
└── compose_override.go # Docker Compose override file generation
```

## The Step Interface

Each step in the wizard implements the `Step` interface defined in `flow.go`:

```go
type Step interface {
    ID() onboardStep
    InputIndex() int
    Question(*Model) string
    Description(*Model) string
    Default(*Model) string
    Validate(*Model, string) error
    Apply(*Model, string)
    Help(*Model) string
    ShouldSkip(*Model) bool
    Clear(*Model)
}
```

### Method Responsibilities

- **`ID()`**: Returns the unique step identifier (e.g., `stepServiceName`)
- **`InputIndex()`**: Returns which text input field to use (0-7), or `-1` for non-input steps
- **`Question()`**: The main prompt shown to the user
- **`Description()`**: Additional explanatory text below the question
- **`Default()`**: Default value if user presses Enter without typing
- **`Validate()`**: Validates user input; returns `nil` if valid, `error` if invalid
- **`Apply()`**: Updates the model with validated input
- **`Help()`**: Footer help text (back button is added automatically by the view)
- **`ShouldSkip()`**: Whether to skip this step based on current state
- **`Clear()`**: Clears this step's state when navigating backward

### BaseStep

Most methods have sensible defaults provided by `BaseStep`. Embed it and override only what you need:

```go
type BaseStep struct{}

func (BaseStep) Validate(*Model, string) error { return nil }
func (BaseStep) Apply(*Model, string)          {}
func (BaseStep) Help(*Model) string            { return "enter: use default • esc: quit" }
func (BaseStep) ShouldSkip(*Model) bool        { return false }
func (BaseStep) Clear(*Model)                  {}
```

## Step Types

### 1. Info Steps (No Input)

Steps that display information and wait for Enter:

```go
type IntroStep struct{ BaseStep }

func (IntroStep) ID() onboardStep        { return stepIntro }
func (IntroStep) InputIndex() int        { return -1 }  // No input needed
func (IntroStep) Question(*Model) string { return "Welcome message" }
func (IntroStep) Description(*Model) string { return "Press [enter] to continue." }
func (IntroStep) Help(*Model) string     { return "enter: continue • esc: quit" }
```

### 2. Yes/No Steps

Steps that wait for `y` or `n` keypresses (handled in `update.go`):

```go
type DockerSetupStep struct{ BaseStep }

func (DockerSetupStep) ID() onboardStep        { return stepDockerSetup }
func (DockerSetupStep) InputIndex() int        { return -1 }
func (DockerSetupStep) Question(*Model) string { return "Will you use Docker? (y/n)" }
func (DockerSetupStep) Help(*Model) string     { return "y: yes • n: no • esc: quit" }
func (DockerSetupStep) Clear(m *Model)         { m.UseDocker = false }
```

### 3. Text Input Steps

Steps that collect text input from the user:

```go
type ServiceNameStep struct{ BaseStep }

func (ServiceNameStep) ID() onboardStep           { return stepServiceName }
func (ServiceNameStep) InputIndex() int           { return 1 }  // Uses inputs[1]
func (ServiceNameStep) Question(*Model) string    { return "What's your service name?" }
func (ServiceNameStep) Description(*Model) string { return "e.g., \"acme-backend\"" }
func (ServiceNameStep) Default(m *Model) string   { return m.serviceNameDefault() }
func (ServiceNameStep) Apply(m *Model, v string)  { m.ServiceName = v }
func (ServiceNameStep) Clear(m *Model)            { m.ServiceName = "" }
```

### 4. Conditional Steps

Steps that are skipped based on state:

```go
type StopCommandStep struct{ BaseStep }

func (StopCommandStep) ShouldSkip(m *Model) bool { return !m.UseDocker }
// ... other methods
```

## Adding a New Step

### Step 1: Define the Step Constant

Add your step to the `onboardStep` enum in `model.go`:

```go
const (
    stepValidateRepo onboardStep = iota
    stepIntro
    // ... existing steps
    stepYourNewStep  // Add here
    stepConfirm
    stepDone
)
```

### Step 2: Create the Step Type

Add your step implementation to `steps.go`:

```go
type YourNewStep struct{ BaseStep }

func (YourNewStep) ID() onboardStep { return stepYourNewStep }
func (YourNewStep) InputIndex() int { return 8 }  // Next available index, or -1
func (YourNewStep) Question(*Model) string {
    return "Your question here?"
}
func (YourNewStep) Description(*Model) string {
    return "Additional context or examples"
}
func (YourNewStep) Default(*Model) string {
    return "default-value"
}
func (YourNewStep) Validate(_ *Model, v string) error {
    // Validate input
    if v == "" {
        return fmt.Errorf("Value cannot be empty")
    }
    return nil
}
func (YourNewStep) Apply(m *Model, v string) {
    m.YourNewField = v
}
func (YourNewStep) Clear(m *Model) {
    m.YourNewField = ""
}
```

### Step 3: Add State to Model

Add the field to store your step's data in `model.go`:

```go
type Model struct {
    // ... existing fields
    YourNewField string
}
```

### Step 4: Register the Step

Add your step to the flow in `steps.go`'s `stepsList()`:

```go
func stepsList() []Step {
    return []Step{
        ValidateRepoStep{},
        IntroStep{},
        // ... existing steps
        YourNewStep{},  // Add in the desired order
        ConfirmStep{},
        DoneStep{},
    }
}
```

### Step 5: Use the Value in Config

If your step's value should be saved to the config file, update `getCurrentConfig()` in `config.go`:

```go
func (m *Model) getCurrentConfig() Config {
    cfg := Config{
        // ... existing config
        YourNewField: m.YourNewField,
    }
    return cfg
}
```

### Step 6: (Optional) Add Custom View Logic

If your step needs special rendering beyond the default, add a case in `view.go`:

```go
case stepYourNewStep:
    // Custom rendering logic
    body.WriteString("Special formatting for your step\n")
```

### Step 7: (Optional) Handle Special Input

If your step uses custom key handling (like `y`/`n`), add logic in `update.go`:

```go
case "y":
    switch m.flow.Current(m.stepIdx).ID() {
    case stepYourNewStep:
        // Handle 'y' keypress
        m.advance()
        return m, nil
    }
```

## Input Field Management

The wizard pre-allocates text input fields for efficiency. Each input step must declare which field it uses via `InputIndex()`.

**Current allocation**: Inputs 0-7 are used. If adding a new input step, use the next available index (8, 9, etc.).

The number of inputs is calculated dynamically in `run.go` based on the maximum `InputIndex()` across all steps.

## Navigation & History

### Forward Navigation

- User presses Enter → `commitCurrent()` validates and applies input → `advance()` moves to next step
- Steps with `ShouldSkip() == true` are automatically skipped by `Flow.NextIndex()`

### Backward Navigation

- User presses Ctrl+B or Left Arrow → pops from history → `clearValuesFromStep()` resets dependent state
- History is managed in `update.go`'s `advance()` method
- Steps excluded from history: `stepValidateRepo`, `stepConfirm`, `stepDone`

### Auto-Skip Logic

When a step is auto-skipped (e.g., Docker type when only one Docker option exists), use manual `stepIdx` increment instead of `advance()` to avoid adding it to history:

```go
m.advance()  // Adds current step to history
m.stepIdx = m.flow.NextIndex(m.stepIdx, m)  // Skip without history
```

## State Clearing

When navigating backward, `clearValuesFromStep()` in `helpers.go` resets the state of the target step and all state that comes after it. This ensures users can change earlier decisions without stale data.

## Viewport Scrolling

The confirmation step (`stepConfirm`) uses a Bubble Tea viewport to allow scrolling through the YAML config preview. This is managed in:

- `model.go`: `viewport` field and `viewportReady` flag
- `update.go`: `updateViewportSize()` and viewport key handling
- `view.go`: Viewport rendering and content setting

## Testing Locally

Run the wizard with:

```bash
cd /path/to/your/nodejs/project
tusk init
```

The wizard will:

1. Validate you're in a Node.js project (has `package.json`)
2. Guide you through configuration
3. Generate `.tusk/config.yaml`
4. Optionally create `docker-compose.tusk-override.yml`

## Common Patterns

### Dynamic Defaults

Use model state to compute defaults:

```go
func (s ServiceNameStep) Default(m *Model) string {
    if m.DockerAppName != "" {
        return m.DockerAppName
    }
    return inferServiceNameFromDir()
}
```

### Dynamic Questions/Descriptions

Tailor prompts based on state:

```go
func (s StartCommandStep) Question(m *Model) string {
    if m.DockerType == dockerTypeCompose {
        return "What's your Docker Compose start command?"
    }
    return "How do you start your service?"
}
```

### Validation with Custom Errors

```go
func (ServicePortStep) Validate(_ *Model, v string) error {
    _, err := strconv.Atoi(trimFirstToken(v))
    if err != nil {
        return fmt.Errorf("Invalid port: must be an integer")
    }
    return nil
}
```

### Conditional Skipping

```go
func (DockerTypeStep) ShouldSkip(m *Model) bool {
    return !m.UseDocker  // Skip if not using Docker
}
```

## Troubleshooting

### Blank screen on a step

- Check that `View()` returns content for your step
- Ensure `InputIndex()` returns a valid index if it's an input step
- Verify the step isn't being auto-skipped unintentionally

### Input not appearing

- Confirm `InputIndex()` returns a valid index (0-7, or higher if you added more)
- Check that the input is focused in `advance()` via `focusActiveInput()`

### Validation not working

- Ensure `Validate()` returns an error (not `nil`) for invalid input
- Check that `commitCurrent()` is being called (happens on Enter for input steps)

## Todos

- [ ] Service compatibility check to support more languages and SDKs
- [ ] Onboarding wizard currently does not support word wrapping for narrower windows
