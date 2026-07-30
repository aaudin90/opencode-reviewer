

# opencode-reviewer

Pipeline automatizado de revisión de código impulsado por [OpenCode](https://opencode.ai). Ejecuta sesiones de revisión con LLM en paralelo contra un diff de rama de Git y publica una revisión consolidada en Markdown.

## Cómo Funciona

1. Obtiene y hace checkout de la rama objetivo.
2. Resuelve la configuración basada en archivos desde la rama realizada en checkout.
3. Genera un diff contra la rama base y lo escribe en `.opencode-review/diff.md`.
4. Inicia una o más sesiones de revisor en paralelo, una por cada mensaje de revisor.
5. Inicia una sesión de finalización y publica la revisión final a través del publicador de VCS configurado.

## Requisitos Previos

- Go 1.26.2
- CLI de [opencode](https://opencode.ai) disponible en `PATH`, o configurado mediante `opencode.binary`
- Acceso a un proveedor de LLM, configurado a través de `provider.json`

## Compilación

```bash
make build
```

Los binarios se generan en `build/opencode-reviewer` y `build/opencode-reviewer-comment-warrior`.

La imagen base de Docker contiene ambas CLIs en `PATH`: `opencode-reviewer` y `opencode-reviewer-comment-warrior`.

## Inicio Rápido

La ruta de configuración principal es un directorio `.opencodereview` local al proyecto.

```bash
./build/opencode-reviewer --config-dir /path/to/repo/.opencodereview --branch my-feature-branch
```

Si `--config-dir` y `OR_CONFIG_DIR` no están configurados, el revisor autodetecta `.opencodereview` en `project_dir` después de hacer checkout de la rama objetivo, o en el directorio de trabajo actual cuando `project_dir` aún no se conoce.

Para desarrollo local con el ejemplo TOML legado:

```bash
make dev-config
make review BRANCH=my-feature-branch
```

La ayuda completa de la CLI está disponible con:

```bash
./build/opencode-reviewer --help
```

Comment-warrior maneja las discusiones existentes de GitLab después de que se haya hecho checkout de la rama:

```bash
./build/opencode-reviewer-comment-warrior --config ./configs/dev.toml --mr-iid 123 --dry-run
```

Ejemplo de comando CI:

```bash
opencode-reviewer-comment-warrior --branch "${CI_MERGE_REQUEST_SOURCE_BRANCH_NAME}" --mr-iid "${CI_MERGE_REQUEST_IID}"
```

## `.opencodereview`

`.opencodereview` es la forma principal y de mayor prioridad para configurar los prompts de revisión y los archivos de proveedor. La configuración anterior basada en variables de entorno para provider/prompt está en desuso y permanece solo como respaldo cuando el modo config-dir está inactivo.

La configuración basada en archivos se lee después de que se prepara la rama objetivo, por lo que `reviewer/messages/*.md` puede ser proporcionado por la rama bajo revisión.

Prioridad del directorio de configuración:

```text
--config-dir flag
  > OR_CONFIG_DIR env
  > <project_dir or cwd>/.opencodereview auto-discovery after checkout
```

Estructura recomendada:

```text
.opencodereview/
  provider.json
  reviewer/
    agent.md
    tools/
      submit_review.ts
      custom_tool.ts
    messages/
      01-bugs.md
      02-security.md
    sub-agents/
      verifier.md
  finalizer/
    agent.md
    message.md
    tools/
      submit_final_review.ts
      custom_tool.ts
    sub-agents/
      verifier.md
  comment-warrior/
    agent.md
    finding-message.md
    mention-message.md
    tools/
      submit_comment_warrior_decision.ts
    sub-agents/
      verifier.md
```

Mapeo de archivos:

| Ruta | Usado como |
|---|---|
| `provider.json` | Configuración del proveedor de OpenCode |
| `reviewer/agent.md` | Prompt del sistema del revisor de la fase 1 |
| `reviewer/messages/*.md` | Mensajes del revisor de la fase 1; los archivos se ordenan lexicográficamente |
| `reviewer/sub-agents/*.md` | Prompts de subagentes del revisor; los archivos se ordenan lexicográficamente |
| `reviewer/tools/*.ts` | Sobrescrituras opcionales de herramientas del revisor o herramientas personalizadas |
| `finalizer/agent.md` | Prompt del sistema del finalizador de la fase 2 |
| `finalizer/message.md` | Mensaje de usuario del finalizador de la fase 2 |
| `finalizer/sub-agents/*.md` | Prompts de subagentes del finalizador; los archivos se ordenan lexicográficamente |
| `finalizer/tools/*.ts` | Sobrescrituras opcionales de herramientas del finalizador o herramientas personalizadas |
| `comment-warrior/agent.md` | Prompt del sistema de comment-warrior |
| `comment-warrior/finding-message.md` | Mensaje de usuario para dar seguimiento a los comentarios de hallazgos de opencode-reviewer |
| `comment-warrior/mention-message.md` | Mensaje de usuario para solicitudes de mención humana `#ai` |
| `comment-warrior/sub-agents/*.md` | Prompts de subagentes de comment-warrior; los archivos se ordenan lexicográficamente |
| `comment-warrior/tools/*.ts` | Sobrescrituras opcionales de herramientas de comment-warrior o herramientas personalizadas |

`submit_review.ts`, `submit_final_review.ts` y `submit_comment_warrior_decision.ts` están incluidos por defecto. Agregue archivos en los directorios `tools` de cada fase solo cuando necesite sobrescribir una herramienta integrada o agregar una herramienta personalizada de OpenCode.

Cuando existen archivos en `.opencodereview`, estos sobrescriben las rutas correspondientes de prompts/proveedores en TOML. Las configuraciones escalares como rama, URL de GitLab y tiempos de espera solo pueden configurarse con un archivo TOML explícito `--config` o variables de entorno. Mantenga el modelo en `provider.json` a menos que intencionalmente necesite una sobrescritura mediante variable de entorno o TOML explícito.
La prioridad de la cadena de modelos de respaldo es `OR_OPENCODE_FALLBACK_MODELS` > `opencode.fallback_models`; el modelo principal proviene de `OR_OPENCODE_MODEL`, `opencode.model` o `provider.json`. Los candidatos de respaldo se verifican antes de que comiencen las sesiones de revisión.

## TOML Explícito

Use `--config some.toml` cuando desee ajustes escalares en TOML:

```toml
project_dir = "/path/to/project"

[env]
  LLM_PROXY_API_KEY = ""

[opencode]
  # Prefer setting model in provider.json.
  # model = "llm-proxy/kimi-k2.5"
  # fallback_models = ["openai/gpt-5", "anthropic/claude-sonnet-4-5"]
  precheck_timeout = 300
  max_steps = 50
  print_logs = false
  # log_level = "DEBUG"
  log_dir = "opencode-review-logs"

[git]
  remote = "origin"
  branch = ""
  base_branch = "main"

[gitlab]
  url = "https://gitlab.example.com"
  token = ""
  project_id = 0
```

Consulte [configs/example.toml](configs/example.toml) para obtener la referencia completa de TOML.

## CLI y Variables de Entorno

Banderas comunes:

| Bandera | Descripción |
|---|---|
| `--config-dir DIR` | Usar un directorio de configuración explícitamente; los archivos se leen después del checkout |
| `--config FILE` | Usar una configuración TOML explícitamente |
| `--branch BRANCH` | Rama a revisar; sobrescribe `OR_BRANCH` y `git.branch` |
| `--review-dump FILE` | Guardar JSON de la revisión final para depuración |
| `--fast-review FILE` | Reproducir un JSON de revisión guardado sin ejecutar las etapas de LLM |

Banderas de comment-warrior:

| Bandera | Descripción |
|---|---|
| `--config FILE` | Usar una configuración TOML explícitamente |
| `--config-dir DIR` | Usar un directorio de configuración explícitamente; los archivos se leen después del checkout |
| `--branch BRANCH` | Rama para checkout; la prioridad es bandera, `OR_BRANCH`, rama de origen CI, TOML |
| `--mr-iid IID` | IID de la solicitud de merge de GitLab |
| `--dry-run` | Compilar tareas y ejecutar el agente sin mutaciones en GitLab |
| `--max-discussions N` | Limitar discusiones procesadas; `0` significa sin límite |
| `--discussion-id ID` | Procesar una discusión de GitLab |
| `--disable-config-dir-auto-discovery` | Deshabilitar la autodetección de `.opencodereview` |

Variables de entorno comunes:

| Variable | Descripción |
|---|---|
| `OR_CONFIG_DIR` | Directorio de configuración usado después del checkout cuando no se establece `--config-dir` |
| `OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY` | Deshabilitar la autodetección de `.opencodereview` con `true` o `1` |
| `OR_PROJECT_DIR` | Ruta del repositorio del proyecto |
| `OR_BRANCH` | Rama a revisar |
| `OR_OPENCODE_MODEL` | Identificador del modelo LLM en formato `provider/model` |
| `OR_OPENCODE_FALLBACK_MODELS` | Modelos de respaldo separados por comas en formato `provider/model` |
| `OR_OPENCODE_PRECHECK_TIMEOUT` | Tiempo de espera para la verificación previa al inicio en segundos |
| `OR_OPENCODE_PRINT_LOGS` | Pasar `--print-logs` a `opencode serve` con `true` o `1` |
| `OR_OPENCODE_LOG_LEVEL` | Pasar `--log-level` a `opencode serve`: `DEBUG`, `INFO`, `WARN` o `ERROR` |
| `OR_OPENCODE_LOG_DIR` | Directorio de registros de OpenCode; las rutas relativas se resuelven bajo `project_dir` |
| `OR_OPENCODE_KEEP_XDG_DIRS` | Mantener los directorios XDG temporales de OpenCode después de la ejecución para depuración con `true` o `1` |
| `OR_GITLAB_URL` | URL de la instancia de GitLab |
| `OR_GITLAB_TOKEN` | Token de acceso privado de GitLab |
| `OR_GITLAB_PROJECT_ID` | ID numérico del proyecto de GitLab |
| `OR_COMMENT_WARRIOR_MR_IID` | IID de MR para comment-warrior |
| `OR_COMMENT_WARRIOR_AGENT_PROMPT_PATH` | Ruta de prompt de agente de comment-warrior de respaldo en desuso cuando config-dir está inactivo |
| `OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH` | Ruta de mensaje de hallazgo de comment-warrior de respaldo en desuso cuando config-dir está inactivo |
| `OR_COMMENT_WARRIOR_MENTION_MESSAGE_PATH` | Ruta de mensaje de mención de comment-warrior de respaldo en desuso cuando config-dir está inactivo |
| `OR_SLOG_LEVEL` | Nivel de registro: `debug`, `info`, `warn`, `error` |

Las variables de entorno de respaldo en desuso como `OR_PROVIDER_CONFIG_PATH`, `OR_PROVIDER_CONFIG`, `OR_AGENT_PROMPT_PATH`, `OR_MESSAGE_PATHS` y las variables de entorno de prompts de finalizador/subagente se ignoran cuando el modo config-dir está activo.

Para artefactos de registro CI:

```yaml
variables:
  OR_OPENCODE_PRINT_LOGS: "true"
  OR_OPENCODE_LOG_LEVEL: "INFO"
  OR_OPENCODE_LOG_DIR: "opencode-review-logs"

artifacts:
  when: always
  paths:
    - opencode-review-logs/
  expire_in: 7 days
```

## Desarrollo

```bash
make build       # compilar binario
make test        # ejecutar pruebas con detector de condiciones de carrera
make linter      # gofmt + golangci-lint + govulncheck + staticcheck + gosec
make deps        # go mod tidy
make dev-config  # crear configs/dev.toml desde el ejemplo
make clean       # eliminar build/
```

Use `NO_PROXY="*"` solo para comandos que realizan llamadas a la red, como descargas de dependencias u operaciones remotas de Git.
