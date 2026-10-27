# Ω omega

**Mantenimiento de macOS guiado por IA.** omega junta lo que hacen
[topgrade](https://github.com/topgrade-rs/topgrade) (actualizar todo el software
instalado) y [Mole](https://github.com/tw93/Mole) (limpiar y optimizar el Mac) en
una sola herramienta de terminal, y pone al mando un modelo de IA que ejecuta
cada paso, **lee los errores, los diagnostica y corrige la configuración**, en
lugar de dejarte el fallo en pantalla.

Con una API key de **Anthropic**, **OpenAI**, **OpenRouter** u **OpenCode Zen**
eliges el modelo que quieres que cuide tu máquina.

```text
Ω omega  anthropic · claude-opus-5-5 · tarea: full
✓ system_info
✓ brew update && brew upgrade · 2m14s
✗ exit 1 pipx upgrade-all
  │ Error: python@3.12 not found
Voy a reinstalar los entornos de pipx contra el Python actual de Homebrew…
? pipx reinstall-all → aprobado
✓ pipx reinstall-all · 41s
✓ brew cleanup --prune=all · 12s
Resumen: 23 paquetes actualizados, 6,4 GB liberados, pipx reparado (nota guardada).
```

---

## Índice

- [Por qué](#por-qué)
- [Características](#características)
- [Requisitos](#requisitos)
- [Instalación](#instalación)
- [Primeros pasos](#primeros-pasos)
- [Tareas](#tareas)
- [Interfaz](#interfaz)
- [Seguridad y aprobaciones](#seguridad-y-aprobaciones)
- [Catálogo de pasos](#catálogo-de-pasos)
- [Memoria entre ejecuciones](#memoria-entre-ejecuciones)
- [Proveedores y modelos](#proveedores-y-modelos)
- [Configuración](#configuración)
- [Referencia de comandos](#referencia-de-comandos)
- [Ejecución desatendida](#ejecución-desatendida)
- [Cómo funciona](#cómo-funciona)
- [Solución de problemas](#solución-de-problemas)
- [Preguntas frecuentes](#preguntas-frecuentes)
- [Desarrollo](#desarrollo)
- [Publicar una versión](#publicar-una-versión)
- [Licencia](#licencia)

---

## Por qué

topgrade y Mole son excelentes, pero cuando algo falla (un `pipx` con el Python
roto, un cask que pide contraseña, un paso que no aplica a tu máquina, una
opción obsoleta en `topgrade.toml`) te toca leer el error, buscar la causa,
editar la configuración y volver a lanzar. omega delega ese ciclo en un modelo
de IA con herramientas acotadas y una política de seguridad estricta.

## Características

- **Actualiza todo**: Homebrew (fórmulas y casks), Mac App Store, npm, pnpm,
  bun, pipx, uv, rustup, cargo, extensiones de `gh`, Oh My Zsh y macOS.
- **Limpia**: Homebrew, cachés de npm/pnpm/pip/uv/Go, Xcode DerivedData,
  simuladores no disponibles, Docker y papelera, priorizando lo que más libera.
- **Usa topgrade y Mole como backends** si están instalados, y puede
  diagnosticar y arreglar su configuración (`omega fix`).
- **Agente con herramientas**: inspecciona el sistema, mide el disco, lee
  configuraciones, ejecuta pasos y comandos, y edita archivos con copia de
  seguridad.
- **Política de seguridad**: cada comando se clasifica (lectura, normal,
  peligroso, bloqueado); lo peligroso requiere tu aprobación.
- **Memoria**: aprende de cada ejecución (p. ej. "este paso falla por X") y lo
  tiene en cuenta la próxima vez.
- **Cuatro proveedores**, cualquier modelo con *tool calling*.
- **TUI a pantalla completa** (Bubble Tea) o **salida de texto** para scripts,
  cron y CI.
- **Registro completo** de cada ejecución en disco.
- **API keys en el llavero de macOS**, nunca en texto plano.
- Binario único en Go, instalable con **Homebrew**.

## Requisitos

- macOS (Apple Silicon o Intel).
- Una API key de al menos un proveedor compatible.
- Opcional: `topgrade` y `mole` (`brew install topgrade mole`).

## Instalación

### Homebrew (recomendado)

```bash
brew install lcajigasm/omega/omega
```

> homebrew-core ya tiene una fórmula llamada `omega` (un motor de búsqueda de
> Xapian), así que usa siempre el nombre completo `lcajigasm/omega/omega`.

### Desde el código fuente

```bash
git clone https://github.com/lcajigasm/omega.git
cd omega
make build          # deja el binario en ./bin/omega
make install        # o lo instala en $GOPATH/bin
```

o directamente:

```bash
go install github.com/lcajigasm/omega/cmd/omega@latest
```

### Autocompletado

La fórmula de Homebrew instala el autocompletado de zsh, bash y fish. Si
compilaste a mano:

```bash
omega completion zsh > "${fpath[1]}/_omega"
```

## Primeros pasos

```bash
omega setup        # elige proveedor, guarda la API key y escoge modelo
omega analyze      # primer contacto: informe sin cambiar nada
omega              # mantenimiento completo
```

`omega setup` muestra los proveedores con su estado, te pide la key con entrada
oculta, la guarda en el llavero de macOS, descarga la lista de modelos del
proveedor y te deja filtrar y elegir uno.

## Tareas

| Tarea | Qué hace | Modifica el sistema |
|---|---|---|
| `full` (por defecto) | revisa el estado, actualiza todo, limpia y resume | sí |
| `update` | solo actualizaciones (equivalente a topgrade) | sí |
| `clean` | mide qué ocupa espacio y limpia (equivalente a `mo clean`) | sí |
| `analyze` | informe de espacio, software desactualizado, `brew doctor` y configuración de topgrade/Mole | **no** |
| `fix` | ejecuta las simulaciones de topgrade y Mole, diagnostica los errores y arregla su configuración | sí (con aprobación) |

También puedes darle una tarea libre:

```bash
omega -p "¿Por qué ocupa 40 GB ~/Library? Dime qué puedo borrar sin riesgo"
omega -p "Actualiza solo Homebrew y npm, nada más"
omega -p "topgrade falla en el paso de rustup, arréglalo"
```

## Interfaz

Por defecto omega abre una TUI a pantalla completa:

- **Cabecera**: proveedor, modelo, tarea y si estás en modo solo lectura.
- **Historial**: mensajes del agente y una línea por comando (`✓`/`✗`, duración);
  si un comando falla verás sus últimas líneas de salida.
- **Panel en vivo**: el comando que se está ejecutando y sus últimas líneas.
- **Confirmaciones**: el comando, su riesgo y el motivo (o el *diff* si edita un
  archivo). Responde con `y` (sí), `a` (sí a todo en esta ejecución) o `n` (no).
- **Seguimiento**: cuando el agente termina puedes pedirle algo más. Enter vacío
  o `Esc` termina la sesión.

| Tecla | Acción |
|---|---|
| `↑` `↓` `PgUp` `PgDn` / rueda | desplazar el historial |
| `y` / `Enter` | aprobar |
| `a` | aprobar esta y todas las siguientes |
| `n` / `Esc` | rechazar |
| `Ctrl+C` | cancelar la ejecución |
| `q` | salir al terminar |

Con `--no-tui`, o si la salida no es un terminal, omega usa salida de texto
simple. Sin terminal en la entrada estándar, cualquier acción que requiera
aprobación se **rechaza** salvo que pases `--yes`.

## Seguridad y aprobaciones

Antes de ejecutar nada, omega clasifica el comando (incluidos los compuestos
con `&&`, `;`, `|`) y aplica el nivel más alto de riesgo:

| Riesgo | Ejemplos | Por defecto |
|---|---|---|
| **lectura** | `brew outdated`, `df -h`, `du`, `cat`, `topgrade --dry-run`, `mo clean --dry-run` | se ejecuta |
| **normal** | `brew upgrade`, `npm update -g`, `pipx upgrade-all`, `rustup update` | se ejecuta |
| **peligroso** | `rm`, `sudo`, redirecciones a archivo, `sed -i`, `find -delete`, editar configs, **cualquier comando desconocido** | pide aprobación |
| **bloqueado** | `rm -rf ~` o `/`, `diskutil erase…`, `dd of=/dev/…`, `curl … \| sh`, `csrutil disable`, `nvram` | nunca se ejecuta |

Ajustes:

- `omega config set approval all`: pide aprobación también para las
  actualizaciones normales.
- `omega config set approval never`, o `--yes`: no pregunta nunca. Los comandos
  bloqueados siguen bloqueados.
- `--dry-run` (o la tarea `analyze`): solo se permiten acciones de lectura; lo
  demás se rechaza automáticamente y el agente lo describe en el resumen.

Otras garantías:

- Los comandos se ejecutan con **stdin cerrado** y variables no interactivas
  (`NONINTERACTIVE=1`, `GIT_TERMINAL_PROMPT=0`…). Lo que intente pedir input
  falla en lugar de colgarse.
- Cada comando tiene un tiempo máximo (`command_timeout`, 45 min por defecto).
- `sudo` se usa siempre como `sudo -n`. Para pasos que lo necesitan, lanza
  `omega --sudo`: pide la contraseña una vez al principio y la mantiene viva
  durante la ejecución.
- `write_file` **siempre** requiere aprobación (salvo `--yes`), muestra el diff y
  guarda una copia `archivo.omega-bak-AAAAMMDD-HHMMSS` del original.
- El agente tiene instrucciones explícitas de no tocar nunca datos personales
  (Documentos, Escritorio, Descargas, Fotos, iCloud, claves SSH, llaveros).

## Catálogo de pasos

`omega steps` muestra qué hay disponible en tu máquina. El agente los usa con
`run_step`, y tú puedes ejecutarlos sin IA con `omega steps run <id>…`.

| id | Categoría | Comando | Riesgo |
|---|---|---|---|
| `disk` | check | `df -h /` + tamaño de cachés, logs y papelera | lectura |
| `brew-outdated` | check | `brew outdated --verbose` | lectura |
| `brew-doctor` | check | `brew doctor` | lectura |
| `macos-check` | check | `softwareupdate --list` | lectura |
| `brew` | update | `brew update && brew upgrade` | normal |
| `brew-greedy` | update | `brew upgrade --cask --greedy` | peligroso |
| `mas` | update | `mas upgrade` | normal |
| `npm` | update | `npm update -g` | normal |
| `pnpm` | update | `pnpm update -g` | normal |
| `bun` | update | `bun upgrade` | normal |
| `pipx` | update | `pipx upgrade-all` | normal |
| `uv` | update | `uv tool upgrade --all` | normal |
| `rustup` | update | `rustup update` | normal |
| `cargo` | update | `cargo install-update -a` | normal |
| `gh-ext` | update | `gh extension upgrade --all` | normal |
| `omz` | update | actualizador de Oh My Zsh | peligroso |
| `macos` | update | `sudo -n softwareupdate --install --all` | peligroso |
| `brew-cleanup` | clean | `brew cleanup --prune=all` | peligroso |
| `brew-autoremove` | clean | `brew autoremove` | peligroso |
| `npm-cache` | clean | `npm cache clean --force` | peligroso |
| `pnpm-store` | clean | `pnpm store prune` | peligroso |
| `pip-cache` | clean | `pip3 cache purge` | peligroso |
| `uv-cache` | clean | `uv cache prune` | peligroso |
| `go-cache` | clean | `go clean -cache` | peligroso |
| `xcode-derived` | clean | vaciar `~/Library/Developer/Xcode/DerivedData` | peligroso |
| `simulators` | clean | `xcrun simctl delete unavailable` | peligroso |
| `docker` | clean | `docker system prune -f` | peligroso |
| `trash` | clean | vaciar `~/.Trash` | peligroso |
| `topgrade-preview` | backend | `topgrade --dry-run` | lectura |
| `topgrade` | backend | `topgrade --yes` | peligroso |
| `mole-clean-preview` | backend | `mo clean --dry-run` | lectura |
| `mole-clean` | backend | `mo clean` | peligroso |
| `mole-optimize-preview` | backend | `mo optimize --dry-run` | lectura |
| `mole-optimize` | backend | `mo optimize` | peligroso |
| `mole-update` | backend | `mo update` | normal |

Para desactivar pasos que nunca quieres que se ejecuten:

```bash
omega config set skip_steps macos,docker,brew-greedy
```

Además del catálogo, el agente puede ejecutar comandos arbitrarios
(`run_command`) para diagnosticar o cuando no hay un paso adecuado, siempre
bajo la misma política de riesgo.

## Memoria entre ejecuciones

Cuando el agente descubre algo duradero de tu máquina (un paso que siempre
falla y su causa, una ruta especial, una preferencia tuya), lo guarda con la
herramienta `remember` en `~/.config/omega/notes.md`. Esas notas se incluyen en
las instrucciones del agente en la siguiente ejecución.

```bash
omega notes          # ver lo aprendido
omega notes clear    # olvidarlo todo
```

El archivo es Markdown normal: puedes editarlo a mano para darle instrucciones
permanentes ("no actualices nunca node, lo gestiono con fnm").

## Proveedores y modelos

| Proveedor | API | Variable de entorno | Modelo por defecto |
|---|---|---|---|
| `anthropic` | Messages API (SDK oficial de Go) | `ANTHROPIC_API_KEY` | `claude-opus-5-5` |
| `openai` | Chat Completions | `OPENAI_API_KEY` | `gpt-5.5` |
| `openrouter` | Chat Completions (compatible con OpenAI) | `OPENROUTER_API_KEY` | `anthropic/claude-opus-5.5` |
| `opencode` | OpenCode Zen: Claude y Qwen Plus/Flash por el endpoint Anthropic; el resto por Chat Completions | `OPENCODE_API_KEY` | `claude-sonnet-4-6` |

- El modelo tiene que soportar **tool calling**.
- Las variables de entorno tienen prioridad sobre el llavero.
- Con Claude puedes ajustar el esfuerzo de razonamiento (`effort`: `low`,
  `medium`, `high`, `xhigh`, `max`). `medium` o `low` suelen bastar para
  mantenimiento rutinario y gastan menos.
- En la API de Anthropic, omega activa el *fallback* del servidor para que una
  negativa del modelo no corte la ejecución.
- Los modelos GPT de OpenCode Zen solo se ofrecen por la Responses API, que
  omega todavía no soporta. Usa GPT con el proveedor `openai` u `openrouter`.

```bash
omega models                       # modelos del proveedor activo (* = actual)
omega models openrouter claude     # filtrar
omega --provider openai --model gpt-5.5 analyze
```

## Configuración

Archivo: `~/.config/omega/config.toml` (`omega config path`). Respeta
`XDG_CONFIG_HOME`.

```toml
provider = "anthropic"          # proveedor activo
model = "claude-opus-5-5"       # modelo del proveedor activo
effort = "medium"               # solo modelos Claude: low|medium|high|xhigh|max
approval = "dangerous"          # dangerous | all | never
language = "es"                 # idioma de las respuestas del agente (es, en…)
command_timeout = "45m"         # tiempo máximo por comando
skip_steps = ["macos", "docker"]

# Modelo y URL base por proveedor (se usan al cambiar con --provider)
[providers.anthropic]
model = "claude-opus-5-5"

[providers.openrouter]
model = "anthropic/claude-opus-5.5"

[providers.opencode]
model = "claude-sonnet-4-6"
base_url = "https://opencode.ai/inference"   # opcional

[providers.openai]
base_url = "https://api.openai.com/v1"       # opcional: proxies o gateways compatibles
```

```bash
omega config show
omega config set provider openrouter
omega config set model anthropic/claude-sonnet-5.5
omega config set effort low
omega config edit                  # abre $EDITOR
```

### Archivos

| Ruta | Contenido |
|---|---|
| `~/.config/omega/config.toml` | configuración |
| `~/.config/omega/notes.md` | memoria del agente |
| `~/.local/state/omega/runs/*.log` | registro de cada ejecución (comandos, salida, decisiones, tokens) |
| Llavero de macOS, servicio `omega-cli` | API keys |

## Referencia de comandos

```text
omega [tarea] [flags]
```

| Flag | Descripción |
|---|---|
| `--provider` | `anthropic`, `openai`, `openrouter`, `opencode` |
| `-m, --model` | modelo a usar en esta ejecución |
| `--effort` | esfuerzo para modelos Claude |
| `-p, --prompt` | tarea libre en lenguaje natural |
| `-y, --yes` | aprobar todo salvo comandos bloqueados |
| `-n, --dry-run` | modo solo lectura |
| `--sudo` | pedir sudo al inicio y mantenerlo |
| `--no-tui` | salida de texto simple |
| `--once` | terminar cuando el agente acabe, sin pedir más instrucciones |
| `-v, --version` | versión |

| Subcomando | Descripción |
|---|---|
| `omega setup` | asistente de configuración |
| `omega auth set <proveedor>` | guardar una API key en el llavero |
| `omega auth remove <proveedor>` | borrarla |
| `omega auth status` | ver qué proveedores tienen key y de dónde sale |
| `omega models [proveedor] [filtro]` | listar modelos |
| `omega config show \| path \| edit \| set <clave> <valor>` | configuración |
| `omega steps` | catálogo de pasos y su disponibilidad |
| `omega steps run <id>… [-y]` | ejecutar pasos sin IA |
| `omega notes [clear]` | memoria del agente |
| `omega completion zsh\|bash\|fish` | script de autocompletado |

## Ejecución desatendida

Para lanzarlo de forma periódica sin interacción:

```bash
omega update --yes --no-tui --once
```

Ejemplo con `launchd` (todos los lunes a las 9:00), en
`~/Library/LaunchAgents/com.lcajigasm.omega.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.lcajigasm.omega</string>
  <key>ProgramArguments</key>
  <array>
    <string>/opt/homebrew/bin/omega</string>
    <string>update</string>
    <string>--yes</string>
    <string>--no-tui</string>
    <string>--once</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict><key>Weekday</key><integer>1</integer><key>Hour</key><integer>9</integer></dict>
  <key>StandardOutPath</key><string>/tmp/omega.out</string>
  <key>StandardErrorPath</key><string>/tmp/omega.err</string>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.lcajigasm.omega.plist
```

Con `--yes` el agente puede ejecutar acciones peligrosas sin preguntar (nunca
las bloqueadas). Si prefieres que en modo desatendido solo actualice, combina
`--yes` con `skip_steps` o usa `-p` con instrucciones precisas.

## Cómo funciona

```text
            ┌──────────────┐  eventos   ┌────────────────────┐
  tarea ──▶ │    agente    │ ─────────▶ │ TUI / texto simple │ ──▶ tú
            │ (bucle + IA) │ ◀───────── │  (aprobaciones)    │
            └──────┬───────┘ decisiones └────────────────────┘
                   │ herramientas
   ┌───────────────┼──────────────────────────────────────┐
   │ system_info  list_steps  run_step  run_command       │
   │ disk_usage   read_file   write_file  remember        │
   └───────────────┬──────────────────────────────────────┘
                   │ clasificador de riesgo (safety)
                   ▼
           runner: zsh -lc, stdin cerrado, timeout, salida en vivo
```

1. omega envía al modelo unas instrucciones de sistema (rol, flujo de trabajo,
   reglas, pasos desactivados y notas de ejecuciones anteriores) y la tarea.
2. El modelo responde con texto y/o llamadas a herramientas.
3. Cada llamada pasa por la política de aprobación: se ejecuta, se te pregunta
   o se rechaza. El resultado (código de salida y salida recortada a ~10 KB) se
   devuelve al modelo.
4. El bucle sigue hasta que el modelo deja de pedir herramientas. En modo
   interactivo puedes continuar la conversación.

Estructura del código:

```text
cmd/omega/          punto de entrada
internal/cli/       comandos cobra (setup, auth, models, config, steps, notes)
internal/agent/     bucle del agente, herramientas, prompt, eventos, registro
internal/llm/       proveedores: anthropic (SDK oficial), openai-compat, opencode
internal/safety/    clasificador de riesgo de comandos
internal/runner/    ejecución de comandos con salida en vivo
internal/steps/     catálogo de pasos
internal/ui/        TUI (Bubble Tea) y salida de texto
internal/config/    configuración y rutas
internal/secrets/   llavero de macOS
Formula/omega.rb    fórmula de Homebrew (se copia al tap)
```

## Solución de problemas

**`no hay API key configurada`**: ejecuta `omega setup` o
`omega auth set <proveedor>`, o exporta la variable de entorno.

**`API key inválida (401)`**: comprueba la key con `omega auth status`. Si
tienes una variable de entorno exportada, tiene prioridad sobre el llavero.

**`modelo o endpoint no encontrado (404)`**: el modelo no existe para ese
proveedor. Lístalos con `omega models <proveedor>`.

**El modelo no llama a herramientas o responde raro**: usa un modelo con *tool
calling*. En OpenRouter, filtra por modelos que lo soporten.

**Un paso con sudo falla con "a password is required"**: relanza con
`omega --sudo`.

**Un comando se queda colgado**: no debería, porque stdin está cerrado, pero si
un programa ignora eso, el `command_timeout` lo corta. Bájalo con
`omega config set command_timeout 15m`.

**Quiero ver exactamente qué hizo**: revisa el último archivo en
`~/.local/state/omega/runs/`.

**El agente insiste en algo que no quiero**: recházalo (`n`) y no lo repetirá en
esa ejecución. Para que sea permanente, añádelo a `skip_steps` o escríbelo en
`~/.config/omega/notes.md`.

## Preguntas frecuentes

**¿Sustituye a topgrade y Mole?** Para lo habitual en un Mac de desarrollo, sí:
tiene pasos nativos equivalentes. Si los tienes instalados, los usa como
backends y te ayuda a arreglar su configuración.

**¿Envía mis datos a la IA?** Se envían al proveedor que elijas las
instrucciones, la salida de los comandos que ejecuta (recortada) y los archivos
que el agente lee (configuraciones). No se envían tus documentos salvo que el
agente lea uno explícitamente, cosa que tiene instruido no hacer.

**¿Cuánto cuesta?** Depende del modelo y de lo que haya que hacer. Al terminar,
omega muestra los tokens usados. Un `update` rutinario suele ser corto; usa
`effort = "low"` o un modelo más pequeño para abaratarlo.

**¿Funciona en Linux?** Está pensado para macOS (llavero, `softwareupdate`, Mac
App Store, rutas de Xcode). Partes del código funcionarían en Linux, pero no
está soportado.

## Desarrollo

```bash
make build     # compila ./bin/omega
make test      # tests (clasificador de riesgo y bucle del agente con un proveedor simulado)
make lint      # gofmt + go vet
```

Para añadir un paso, edita `internal/steps/steps.go`. Para añadir un proveedor,
implementa `llm.Provider` en `internal/llm/` y regístralo en `llm.New` y
`llm.Providers`.

## Publicar una versión

```bash
git tag v0.1.0 && git push origin v0.1.0
make formula VERSION=0.1.0          # actualiza url y sha256 en Formula/omega.rb
cp Formula/omega.rb ../homebrew-omega/Formula/omega.rb
cd ../homebrew-omega && git commit -am "omega 0.1.0" && git push
```

## Licencia

[MIT](LICENSE) © Luis Cajigas
