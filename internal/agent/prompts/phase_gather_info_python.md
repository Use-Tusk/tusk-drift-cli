## Phase: Gather Project Information (Python)

Collect the details needed to set up Tusk Drift for this Python project.

### Required Information

| Field | How to find | Example |
|-------|-------------|---------|
| package_manager | Lockfile: requirements.txt → pip, pyproject.toml with poetry.lock → poetry, Pipfile.lock → pipenv, uv.lock → uv | "pip" |
| framework | Check imports/dependencies: flask, fastapi, django, etc. | "fastapi" |
| entry_point | Look for main.py, app.py, wsgi.py, asgi.py, or check pyproject.toml scripts | "app/main.py" |
| start_command | Check pyproject.toml scripts, Makefile, or common patterns | "uvicorn app.main:app --reload" |
| port | Look for PORT env var or hardcoded port in entry file | "8000" |
| health_endpoint | Grep for /health, /healthz, /liveness routes | "/health" |
| docker_type | Check for Dockerfile or docker-compose.yml | "none" |
| service_name | pyproject.toml "name" or directory name | "my-service" |

### Instructions

1. **Package Manager**: Check which files exist:
   - `requirements.txt` alone → pip
   - `pyproject.toml` + `poetry.lock` → poetry
   - `pyproject.toml` + `uv.lock` → uv
   - `Pipfile` + `Pipfile.lock` → pipenv
   - `pyproject.toml` alone → pip (with pyproject.toml)

2. **Framework**: Read dependencies to detect:
   - Flask: `flask` in dependencies
   - FastAPI: `fastapi` in dependencies
   - Django: `django` in dependencies
   - Other WSGI/ASGI frameworks

3. **Entry Point**:
   - Check pyproject.toml for `[project.scripts]` or `[tool.poetry.scripts]`
   - Look for common entry files: `main.py`, `app.py`, `wsgi.py`, `asgi.py`, `app/__init__.py`
   - For Django: look for `manage.py` and `wsgi.py`/`asgi.py`

4. **Start Command**:
   - FastAPI/Starlette: `uvicorn app.main:app --host 0.0.0.0 --port 8000`
   - Flask: `flask run --host 0.0.0.0 --port 5000` or `python app.py`
   - Django: `python manage.py runserver 0.0.0.0:8000`
   - Check Makefile, pyproject.toml scripts, or Procfile for hints
   - If multiple options exist, ask the user

5. **Port**:
   - Grep for `PORT` or `os.environ.get("PORT")`
   - Check entry file for hardcoded port
   - Default: 8000 for FastAPI/Django, 5000 for Flask

6. **Health Endpoint**:
   - Grep for common patterns: `/health`, `/healthz`, `/liveness`, `/ready`, `/ping`
   - If not found, note as empty string

7. **Docker**:
   - Set to "dockerfile" if Dockerfile exists
   - Set to "compose" if docker-compose.yml exists
   - Set to "none" if service can run without Docker

8. **Service Name**: Use pyproject.toml "name" or fall back to directory name

### Edge Cases

- **Monorepo**: If you see multiple pyproject.toml files, ask user to confirm the service root
- **Virtual Environment**: Note if `.venv`, `venv`, or `.virtualenv` directory exists
- **Django projects**: These have a specific structure with settings.py, manage.py, and app directories

### Transition

Call `transition_phase` with all gathered information:

```json
{
  "results": {
    "package_manager": "pip",
    "framework": "fastapi",
    "entry_point": "app/main.py",
    "start_command": "uvicorn app.main:app --host 0.0.0.0 --port 8000",
    "port": "8000",
    "health_endpoint": "/health",
    "docker_type": "none",
    "service_name": "my-service"
  }
}
```
