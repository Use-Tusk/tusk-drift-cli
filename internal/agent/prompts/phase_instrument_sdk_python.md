## Phase: Instrument SDK (Python)

Install the Tusk Drift Python SDK and instrument the application.

### Step 1: Install SDK (if not already installed)

First check if `tusk-drift-python-sdk` is already in the project's dependencies.
If NOT installed, run the install command based on the package manager:

- pip: `pip install tusk-drift-python-sdk`
- poetry: `poetry add tusk-drift-python-sdk`
- pipenv: `pipenv install tusk-drift-python-sdk`
- uv: `uv add tusk-drift-python-sdk`

With framework extras (recommended):

- Flask: `pip install tusk-drift-python-sdk[flask]`
- FastAPI: `pip install tusk-drift-python-sdk[fastapi]`

Skip installation if the SDK is already in dependencies.

### Step 2: Create SDK Initialization File

Create a file called `tusk_drift_init.py` in the project root or next to the entry point.
IMPORTANT: All code files must end with a trailing newline.

NOTE: This is LOCAL setup - do NOT use any API keys. Leave api_key as None for local mode.

```python
from drift import TuskDrift

TuskDrift.initialize(
    env="development",
)
```

### Step 3: Import SDK at Application Startup

The SDK must be initialized BEFORE the application framework is imported.

**For FastAPI:**

Modify the entry file (e.g., `main.py` or `app.py`):

```python
# Initialize Tusk Drift SDK FIRST - before any other imports
import tusk_drift_init  # noqa: F401

from fastapi import FastAPI

app = FastAPI()

# After creating the app and registering routes:
from drift import TuskDrift
TuskDrift.get_instance().mark_app_as_ready()
```

**For Flask:**

```python
# Initialize Tusk Drift SDK FIRST
import tusk_drift_init  # noqa: F401

from flask import Flask

app = Flask(__name__)

# At the end of app setup (before run):
from drift import TuskDrift
TuskDrift.get_instance().mark_app_as_ready()

if __name__ == "__main__":
    app.run()
```

**For Django:**

Add to `manage.py` and/or `wsgi.py`/`asgi.py` at the very top:

```python
#!/usr/bin/env python
# Initialize Tusk Drift SDK FIRST
import tusk_drift_init  # noqa: F401

import os
import sys
# ... rest of manage.py
```

For Django, call `mark_app_as_ready()` in a signal or AppConfig.ready():

```python
# In your_app/apps.py
from django.apps import AppConfig

class YourAppConfig(AppConfig):
    name = "your_app"

    def ready(self):
        from drift import TuskDrift
        TuskDrift.get_instance().mark_app_as_ready()
```

### Step 4: Mark App as Ready

The SDK needs to know when the app is ready to receive requests.

For all frameworks, ensure `TuskDrift.get_instance().mark_app_as_ready()` is called after:

- The app object is created
- Routes are registered
- Middleware is configured
- Just before the server starts listening

When done, call transition_phase with:

```json
{
  "results": {
    "sdk_installed": true,
    "sdk_instrumented": true
  }
}
```
