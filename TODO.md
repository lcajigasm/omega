# TODO: cuarta revisión de omega (60 ejecuciones reales, 19–23 nov 2026)

Revisión del 23 de noviembre de 2026 sobre `main` en `5a5d546` (v0.30.0), a
partir de las 60 transcripciones de `~/Desktop/Runs` (19, 22 y 23 de
noviembre; 54 con Haiku y 6 con Sonnet; 12 cambian a Sonnet a mitad; $2,97
en total). Las ejecuciones vienen de **dos Macs** que escriben en la misma
carpeta (un Mac mini de 228 GB con `~/Dev/doctorSIM` y un Mac Studio de
460 GB con `~/Dev/Deudafix`); el registro no dice cuál es cuál.

Los criterios son los de las revisiones anteriores. Un P0 hace algo que el
usuario no ha aprobado, borra datos o deja rota una función principal. Un
P1 da un resultado equivocado, engaña, cuesta tiempo o dinero sin motivo, o
impide revisar lo que pasó.
Cada punto dice qué pasó (con la ejecución, `AAAA-MM-DD_HHMMSS.log:línea`
cuando ayuda), la causa en el código cuando se ha encontrado, y qué tiene que
pasar.

## P0

### A. Decisiones que nadie tomó

- [x] **A1. En `--unattended`, `propose_actions` elige todas las opciones,
  también las que se excluyen entre sí, y le dice al modelo que las eligió
  el usuario.** Es el «selector que devuelve todas las opciones» que
  aparece en 9 ejecuciones (19_185015, 19_185202, 19_190346, 19_190543,
  19_191120, 19_112126, 22_150146, 22_150714, 23_082253). Llegaron a la vez
  «aplicar ControlMaster» y «no cambiar nada», tres dueños para ⌥Space, y en
  19_191120 «borrar los 80 volúmenes activos de Docker (~87 GB)», propuesto
  por el propio agente. En 22_150146 el agente actuó sobre la respuesta:
  `rm -rf` de `node_modules` y de `~/.cargo/registry/src` y `mo purge --yes`
  (4,17 GB). Los resúmenes dicen «las que elegiste», y el usuario no eligió
  nada.
  Causa: `toolProposeActions` (`internal/agent/tools.go:802`) toma todas
  las opciones cuando `a.opts.Unattended` y contesta `The user selected:`.
  Debe:
  - con `multiple=false`, no elegir ninguna (o solo la marcada como
    recomendada) y decirlo;
  - no tomar nunca una opción de «no cambiar nada» ni una de riesgo alto o
    irreversible;
  - contestar «elegido automáticamente (ejecución desatendida); nadie
    eligió», para que el modelo no lo atribuya al usuario;
  - rechazar en la capa de la herramienta una respuesta de elección única
    con más de un id, también en la TUI y en la app.
  Ahora: en desatendido, una pregunta de elección única solo toma la opción
  marcada `recommended` (nueva en la herramienta), y ninguna si no hay una;
  una de varias toma las que no son `dangerous` ni de «no cambiar nada». La
  respuesta dice «UNATTENDED RUN: nobody saw this card and nobody chose» y
  pide no atribuirlo al usuario. Una tarjeta de elección única que vuelve
  con varias respuestas no se ejecuta. Pruebas en
  `internal/agent/review4_test.go`.
- [x] **A2. El agente no sabe en qué modo está y a veces decide ignorarlo.**
  Las ejecuciones guiadas se tratan como desatendidas, y al revés:
  - «lo elegiste, pero en esta ejecución desatendida… se ha saltado»
    (19_111855:117, 19_185353:61, 22_145951:46, 23_082014:36);
  - 23_082253:7: «la salida dice 'UNATTENDED RUN', pero tú pediste una
    revisión guiada, así que sigo tus instrucciones y no las restricciones
    de ese texto».

  El agente desobedece así una barrera de omega por su cuenta.
  Debe: un modo único y explícito por sesión. Va en el prompt y en cada
  rechazo, y se ve en la cabecera del registro (ver C1). El prompt
  desatendido prohíbe expresamente reinterpretarlo. Si no hay nadie
  delante, no se ofrecen tarjetas que luego se van a rechazar.
  Ahora: el prompt lleva una línea «Run mode (set by omega;
  authoritative…)» construida con las opciones de la sesión (desatendido,
  etapa guiada, interactivo, no interactivo), el texto desatendido prohíbe
  reinterpretarlo, y el modo va en la cabecera del registro. En desatendido
  no se toma ninguna opción peligrosa, así que no se ofrecen tarjetas que
  luego se rechacen.
- [x] **A3. `run_command` deja pasar lo que los pasos del catálogo no.**
  Mientras `docker builder prune` se bloquea en 9 ejecuciones, pasan sin
  tarjeta ni registro de aprobación:
  - `rm -rf ~/Library/Application Support/Claude/vm_bundles` (11 GB,
    definitivo, 19_184014:394, 23_081643:204);
  - `rm -rf` de cachés de Spotify, que perdió las descargas sin conexión
    (22_145619:124, 23_081220:160, 19_190020:135), de Chrome,
    `codex-runtimes` y `ms-playwright-go`;
  - `git worktree remove` en 15 worktrees (19_105805:14835), en otros 3
    (19_184730:220) y en 2 más (23_080707:775), esta última en una
    ejecución que había dicho «Solo haré lecturas»;
  - `sudo socketfilterfw --setstealthmode on` (19_184355:114);
  - `brew reinstall --cask --force alfred`, `brew uninstall --cask --force`;
  - `git config --global --unset maintenance.repo` (19_191120:85), después
    de diagnosticar mal el remoto como inaccesible;
  - `go env -w GOFLAGS=…` (22_150034:202);
  - `sudo mas upgrade` (23_081230:162).

  Debe:
  - pedir siempre aprobación para `rm -rf` sobre `~/Library` y `~/Dev`,
    `git worktree remove`, `git config --global`, `go env -w`,
    `defaults write`, `brew … --force` y `sudo …`, igual que el paso
    equivalente;
  - en una ejecución desatendida, saltarlos;
  - los borrados van a la Papelera de omega, no con `rm -rf`, y antes se
    comprueba que la app no está abierta;
  - los cambios persistentes de configuración pasan por `mac_change` o el
    asistente, con copia y `undo`.
  Ahora: `safety.AlwaysConfirm` (`internal/safety/ask.go`) pregunta siempre
  por `rm -r` de datos de apps y de contenido del usuario (no de cachés
  regenerables, Papelera ni temporales; la caché de Spotify sí pregunta),
  `git worktree remove`, `git config --global` que escribe, `go env -w/-u`,
  `defaults write/delete`, `brew … --force`/`--zap`, cualquier `sudo`
  (salvo `-n true`, `-v`, `-k`, `-l`) y `mo purge`. En desatendido se
  saltan. El prompt pide mover a la Papelera tras comprobar que la app no
  está abierta, y aplicar ajustes duraderos con `write_file`, `mac_change`
  o el asistente. Cada aprobación queda en el registro con quién la dio.
- [x] **A4. El agente se salta la confirmación de otra herramienta.**
  `mo purge` contestó «Purge requires confirmation… --yes to confirm» y el
  agente lo relanzó con `--yes` sin preguntar (22_150146:163-172). Debe:
  añadir `--yes`, `-f` o `--force` después de que la herramienta se negara
  es «confirmar siempre». `mo purge` pasa a ser un paso del catálogo con su
  propia propuesta.
  Ahora: si la salida de un comando pide una confirmación propia («--yes to
  confirm», «requires confirmation»), añadir después `--yes`, `-y` o
  `--force` a ese programa es «confirmar siempre», también con `--yes` de
  omega; `mo purge` y `mo` con `--yes` preguntan siempre.
- [x] **A5. El agente ofrece opciones catastróficas.** Llegó a proponer
  borrar volúmenes de Docker en uso con datos de Postgres (19_191120:104);
  con A1, se habrían «elegido». Debe: «confirmar siempre» para
  `docker volume rm` o `prune` sobre volúmenes en uso, y el prompt prohíbe
  proponerlo como una opción más.
  Ahora: `docker volume rm/prune`, contenedores y `system prune` siguen en
  «confirmar siempre», y el prompt prohíbe ofrecer borrar volúmenes en uso
  o contenedores parados con datos fuera de un volumen con nombre.
- [x] **A6. `brew upgrade` se lanzó con 3–7 GB libres y llenó el disco.**
  Salieron 13.063 líneas «No space left on device», `exit=1` tras 1 min
  59 s, y un registro de 2,5 MB que costó $0,38, cien veces una ejecución
  normal (19_105805:536-14545). Debe:
  - antes de `brew`, `brew-greedy` y `topgrade`, comparar el espacio libre
    con lo que se va a descargar (`brew upgrade --dry-run` da tamaños) o con
    un mínimo, por ejemplo 10 GB;
  - por debajo, no ejecutar y decir por qué;
  - si aparece «No space left on device», cortar el paso en lugar de dejar
    que siga.
  Ahora: `brew`, `brew-greedy`, `topgrade`, `mas` y `macos` empiezan
  comprobando el espacio libre del volumen de datos (`omega_space` en
  `internal/steps/shell.go`): con menos de 10 GB (`OMEGA_MIN_FREE_GB`)
  dicen cuánto queda y qué hacer, y salen con 1 sin ejecutar nada.
  Cualquier orden que dé 20 errores «No space left on device» se detiene y
  la respuesta dice que el disco está lleno.

## P1

### B. Política y modos

- [x] **B1. Los pasos `health`, `security` y `system-logs` «están
  bloqueados»** según el agente (19_111023, 19_184355:63, 19_185142:64,
  22_145320:48, 23_080934:48, 23_081159:47). Al bloquearlos, el agente
  rehace sus comandos uno a uno con `run_command`, en 6 ejecuciones y unas
  10 llamadas cada vez. En el código actual el paso pasa con su riesgo
  `Safe`, así que no se ha podido reproducir el bloqueo, y el registro no
  dice el motivo (ver C2).
  Lo que sí falla: el clasificador da «peligroso» a los comandos de esos
  pasos sueltos, así que por `run_command` piden aprobación:
  - `sed 's/^ *//'`
  - `memory_pressure -Q`
  - `softwareupdate --schedule`
  - `socketfilterfw --getglobalstate`
  - `/usr/bin/log show`
  - `echo $(…)`
  - `for …; do echo …; done`

  Debe: averiguar el rechazo con el motivo registrado, y que esos
  comandos sean de solo lectura. Hace falta una prueba de que cada paso de
  categoría `Check` se clasifica `Safe`.
  Ahora: cada comando de los pasos `Check` se clasifica de solo lectura,
  con prueba que los copia literalmente (`TestReview4CheckStepsReadOnly`):
  `sed` sin escritura, `memory_pressure -Q`, `softwareupdate --schedule`,
  `socketfilterfw --get…`, `log show`, `$(…)` y bucles `for/while` con
  cuerpo de solo lectura. Un `$'` dentro de comillas ya no se toma por
  ANSI-C (era lo que hacía peligroso `system-logs`). Los rechazos se
  registran con su motivo (C2), así que un bloqueo futuro se verá.
- [x] **B2. Lecturas bloqueadas en modo desatendido:** el plist de un
  agente de inicio (19_185142:250) y un `grep` de `~/.zshrc`
  (19_190418:154). Debe: leer no es cambiar; solo se bloquean los
  ficheros secretos de siempre.
  Ahora: `plutil -p`, `launchctl print/list`, `grep`/`cat` de `~/.zshrc` y
  de Oh My Zsh, `defaults read`, `lipo -archs`, `xcode-select -p`, `pgrep`,
  `docker info/system df`, `brew uses`, `git worktree list`, `git
  rev-list`… son de solo lectura y `PersonalSetup` no los marca.
- [x] **B3. `docker builder prune` (y `image prune` de imágenes colgantes)
  bloqueado en 9 ejecuciones.** Es caché que se regenera, y el paso
  `containers` de topgrade hace `docker image prune -f` sin preguntar
  (19_185256:189, 23_082342:151). Debe: una sola política para Docker. La
  caché de build y las imágenes colgantes son de riesgo normal; volúmenes,
  contenedores parados e imágenes sin registro, «confirmar siempre».
  Ahora: `docker builder prune` e `image prune` sin `-a` (caché y
  colgantes) ya no son «confirmar siempre» (siguen pidiendo aprobación como
  peligrosos); `-a`, volúmenes, contenedores y `system prune` sí.
- [x] **B4. `remember` falla y dice que funcionó.**
  - El límite de 400 caracteres devuelve error, pero el registro pone
    `[remember exit=0]` (19_105805:15128, 19_185142:218, 23_080707:843,
    23_082106:194) y el resumen dice «Nota guardada» o culpa a la
    aprobación.
  - Tras leer la web la nota pide aprobación, y en desatendido no se guarda
    (`toolRemember`, `tools.go:701`). Parece aleatorio, y el agente no lo
    entiende.

  Debe:
  - el límite en la descripción de la herramienta;
  - `exit≠0` en el registro para cualquier error de herramienta;
  - el rechazo por web, explicado en el resumen con esas palabras.
  Ahora: el límite de 400 caracteres está en la descripción de `remember`;
  el error dice «not saved… Write it again, shorter»; cualquier herramienta
  que falla deja `[tool FAILED]` en el registro; el rechazo tras leer la
  web pide decir en el resumen que no se guardó por eso.
- [x] **B5. `mac_change` y `mac_policy` rechazan lo que el usuario
  eligió** («ha rechazado los cambios que elegiste», 19_185202:13;
  19_112126:25) y el resumen manda a «usar `omega mac`», que es lo que se
  estaba usando. Debe: decir el motivo real del rechazo y no proponer en
  ese modo lo que se va a rechazar.
  Ahora: el rechazo explica que solo ocurre en ejecuciones desatendidas y
  que el usuario puede aplicarlo con `omega mac` en una sesión con alguien
  delante; en desatendido no se ofrecen esos cambios (A1).
- [x] **B6. En desatendido se abren paneles de Ajustes**
  (`open x-apple.systempreferences:…Time-Machine…`, `Privacy_AllFiles`;
  19_111023, 19_184355:105, 23_080934:139, 23_082213:10). Debe: en
  desatendido, `open` de una URL de Ajustes se convierte en un enlace del
  resumen.
  Ahora: en desatendido, `open x-apple.systempreferences:…` no se ejecuta y
  la respuesta da el enlace para el resumen.

### C. El registro de la ejecución

- [x] **C1. La cabecera no dice lo necesario.** Solo tiene la fecha, el
  proveedor y el modelo. Falta:
  - la versión de omega (que se actualizó a sí misma a mitad de
    19_105805:14181);
  - el equipo (nombre, modelo, disco): dos Macs escriben en la misma
    carpeta;
  - la orden y la tarea (`omega full --unattended`, la etapa, o la pregunta
    del usuario);
  - el modo (guiado, desatendido, solo lectura, automático) y si hay
    `--sudo`.
  Ahora: la cabecera lleva, tras la primera línea de siempre: `version`,
  `host` (nombre, modelo, disco y libre), `command` con la tarea, y `mode`
  (desatendido/etapa guiada/interactivo, solo lectura o automático,
  política de aprobación, sudo). La primera entrada es la tarea (`> …`).
- [x] **C2. Los rechazos no se registran.** `authorize` y `toolRemember`
  emiten `EvDenied`, pero `writeLog` (`internal/agent/log.go:157`) no tiene
  ese caso. Ningún «bloqueado por la política», «solo lectura» o «necesita
  una persona» deja rastro en las 60 ejecuciones, y no se puede saber qué
  pasó en B1. Debe: `[run_step:health DENEGADO: <motivo>]`.
  Ahora: `writeLog` escribe `EvDenied` como `[tool DENIED] comando` con
  `reason:`.
- [x] **C3. Las preguntas y las respuestas no se registran.** Ni la
  pregunta de `propose_actions` o `ask`, ni las opciones, ni lo que volvió,
  ni si lo eligió una persona o el modo desatendido (A1). Las aprobaciones
  automáticas tampoco. Debe: registrar la pregunta, las opciones y los ids
  devueltos, con quién decidió.
  Ahora: se registran la pregunta y las opciones (`? choice`), la respuesta
  y quién la dio (`? answer: … — chosen by the user / omega, automatically
  / nobody`), y cada aprobación (`[tool approved by the user]`, `[…
  approved without asking (…)]`).
- [x] **C4. Las herramientas que no son de shell solo dejan
  `[tool exit=0 0s]`:** `system_info`, `read_file`, `fetch_url`,
  `web_search`, `dev_inventory`, `mac_inventory`, `run_history` y
  `remember` (sin el texto de la nota). Los resúmenes citan datos que no
  están en ningún sitio («16 GB de RAM», «zsh arranca en 166 ms», «Go
  1.27.4»). Debe: una línea con los argumentos y un resumen del resultado
  (líneas, bytes, primeras líneas, el texto de la nota).
  Ahora: las herramientas que no son de shell dejan `[tool returned N
  lines]` con sus primeras 15 líneas; `remember` deja el texto de la nota.
- [x] **C5. La salida de las llamadas en paralelo se mezcla línea a
  línea.** Todas las cabeceras `$` salen primero y la salida de cuatro pasos
  después, intercalada, con `[run_step:disk]` cerrando en mitad de
  topgrade. En 23_082106 eso llevó a leer mal `brew uses openjdk@25` y a
  proponer desinstalarlo. Debe: guardar la salida de cada llamada y
  escribirla en bloque bajo su propia cabecera, o prefijar cada línea con
  un id.
  Ahora: dentro de un lote en paralelo, la salida de cada llamada se guarda
  y se escribe en bloque bajo su propia cabecera al terminar.
- [x] **C6. `read_file`, `fetch_url` y `web_search` se escriben como
  `$ /ruta` o `$ https://…`,** como si fueran comandos. Debe: `read_file
  /ruta`, `fetch_url …`, `web_search «…»`.
  Ahora: solo `run_command`, `run_step:*`, `disk_usage` y `mac_change` se
  escriben con `$`; el resto con `» tool argumentos`.
- [x] **C7. El cambio de modelo no dice por qué** (`[model
  claude-sonnet-5-5]`), y el coste final no se reparte por modelo. Tampoco
  se dice por qué termina una sesión: 22_143904:905 acaba con
  `mole-clean exit=-1` y sin resumen. Debe: el motivo (paso que falló,
  cancelación, tiempo agotado, presupuesto) y un resumen parcial aunque
  la sesión se corte.
  Ahora: `[model X: motivo]` (paso que falló o elección del usuario); el
  final reparte el coste por modelo, dice el motivo (cancelado,
  presupuesto) y cuenta las herramientas que funcionaron y las que
  fallaron, aunque no haya resumen; `exit=-1` dice «stopped: timed out or
  cancelled».
- [x] **C8. La salida sin límite infla el registro y los tokens.** Además
  de A6, entran enteros:
  - la tabla de `brew upgrade` dos veces (unas 530 líneas);
  - avisos de rdoc «already initialized constant» (`gem update`);
  - avisos de Spotlight de `mas`;
  - el `topgrade.toml` y el `clean-list.txt` comentados;
  - `docker system df -v` con 90 volúmenes.

  Debe:
  - colapsar líneas repetidas con el mismo prefijo;
  - recortar con cabeza, cola y líneas de error, tanto para el modelo como
    para el registro;
  - filtros por paso: `gem update --no-document` y `MAS_NO_AUTO_INDEX=1`
    o equivalente.
  Ahora: las líneas repetidas con la misma forma se pliegan («… N more
  lines like the one above») para el registro y para el modelo; cada
  llamada escribe como mucho 300 líneas, luego solo errores (30) y las 40
  últimas. Los filtros por paso (`gem --no-document`, avisos de `mas`)
  están en los pasos.

### D. Pasos que fallan o mienten

- [x] **D1. Ningún comando corre con `pipefail`.** `topgrade … | tail
  -150` sale con 0 con «Brew Cask: FALLIDO» (19_111309); `mo purge … |
  tail -4; rm -rf …` sigue tras el rechazo (22_150146:161); `pgrep … | head;
  echo exit=$?` fue la comprobación previa al `rm -rf vm_bundles`
  (19_184014:387). Causa: `runner.go:105` usa `$SHELL -lc` sin
  `pipefail`. Debe: `set -o pipefail` delante (también en zsh), y el prompt
  advierte de que `| tail` oculta el código.
  Ahora: el intérprete corre con `pipefail` (zsh `setopt pipefail`, bash/sh
  `set -o pipefail` si existe) y el prompt advierte de `| tail` y de `echo
  exit=$?` tras una tubería.
- [x] **D2. zsh aborta con un glob sin coincidencias** (`no matches found:
  /Applications/MySQL*`, 19_185555:83; 19_190543:26 mató un bucle y salió
  con 0). Debe: `setopt nullglob` o `nonomatch` en el prefijo del
  intérprete, o `/bin/bash` para `run_command`.
  Ahora: zsh corre con `setopt no_nomatch`: un patrón sin coincidencias
  queda literal, como en bash.
- [x] **D3. Un paso que sale con `≠0` hace subir el modelo a Sonnet**
  (`runShell`, `tools.go:228`). Eso pasa aunque el fallo sea benigno:
  - `brew-doctor` con solo un aviso de `icu4c@78` deprecado;
  - `system-logs` con salida completa;
  - `helm-repos` sin repositorios;
  - `docker-df` con Docker apagado.

  Es lo que encarece 12 ejecuciones (las 4 con Sonnet son el 79 % del coste
  del 19 por la tarde). Debe: corregir esos pasos (abajo) y subir de modelo
  solo ante un fallo que lo justifique, nunca por un aviso o por una
  herramienta ausente.
  Ahora: solo sube de modelo un paso que falla por algo que un modelo mejor
  puede arreglar: Docker parado, nada que actualizar, avisos, disco lleno o
  sin sudo no lo hacen (`benignFailure`). La documentación lo dice.
- [x] **D4. `system-logs` sale con 1 o 0 según la última tubería**
  (19_185142:54, 22_144919:284, 23_080934:38). Debe: `|| true` por sección
  y un código calculado a propósito.
  Ahora: la orden de macOS termina en `; true` (la de Linux ya lo hacía), y
  sus comandos son de solo lectura (B1).
- [x] **D5. `brew-doctor` sale con 1 con solo avisos de fórmulas
  deprecadas.** Debe: «aviso», no «fallo».
  Ahora: un código 1 (solo avisos) pasa a 0 con «(brew doctor found
  warnings: they are advice, not a failure)»; cualquier otro código se
  conserva.
- [x] **D6. `docker-df` falla con Docker apagado** (4 ejecuciones) y el
  agente lanza `docker-start` u `open -a Docker && sleep 40`, y deja
  Docker abierto. Debe: comprobar `docker info`, decir «Docker no está en
  marcha» con salida 0 y medir `Docker.raw` sin arrancarlo.
  Ahora: `docker system df` o «Docker is not running; measured the disk
  image without starting it», y el tamaño de `Docker.raw`; sale con 0 y
  nunca arranca Docker.
- [x] **D7. `helm-repos`, `pnpm update -g` y `uv tool upgrade` fallan o
  hacen ruido sin nada instalado** («Error: no repositories found», «Es lo
  de siempre»), aunque ambos `topgrade.toml` desactivan helm por eso.
  Debe: `Available()` comprueba que hay algo que actualizar.
  Ahora: `helm-repos`, `pnpm` y `uv` solo están disponibles cuando hay algo
  que actualizar (`helm repo list`, `pnpm ls -g`, `uv tool list`, una vez
  por proceso y con 5 s de límite).
- [x] **D8. `topgrade` desatendido falla en el primer paso por sudo, y la
  vista previa dice que todo va bien.** Sin credenciales, `pre_sudo` o
  `sudo_loop` hacen `sudo -v` y fallan con «Failed to elevate
  permissions» (19_105805:14854, 22_143904:314). La vista previa simula
  `sudo -v` y da «Sudo OK» (19_112246, 22_150932:262), y los consejos de
  configuración alaban `pre_sudo=false` sin ver `sudo_loop = true`.
  Debe:
  - sin sudo, lanzar topgrade con `--disable` de los pasos que lo piden;
  - la vista previa avisa «esta configuración necesita sudo y esta
    ejecución no lo tiene»;
  - `config_hints` conoce `sudo_loop`.
  Ahora: topgrade ejecuta `sudo -v` aunque se le pase `--disable`
  (comprobado con topgrade 17.17), así que, si la configuración tiene
  `pre_sudo` o `sudo_loop = true`, el paso usa un `--config` temporal que
  incluye la del usuario y sus ficheros de `topgrade.d` y apaga los dos al
  final; los pasos que necesitan sudo lo piden al ayudante de omega o
  fallan solos. La vista previa avisa de que esa configuración falla sin
  sudo. `config_hints` explica `pre_sudo` y `sudo_loop`.
- [x] **D9. Casks que necesitan sudo o cuya app ya no está** (naps2, obs,
  busycal, mysqlworkbench, Alfred 5→6) hacen fallar `brew` y `brew-greedy`
  una ejecución tras otra (`brew-greedy` tardó 3 min 14 s sabiendo que uno
  fallaría, 19_185555). Debe: antes del paso, detectar los casks sin app
  o que piden sudo, apartarlos y proponer su arreglo
  (`uninstall --force` o reinstalar) o `--sudo`.
  Ahora: `brew` y `brew-greedy` leen el recibo de cada cask: dicen cuáles
  están instalados sin su app, con la orden para arreglarlos (`brew
  reinstall --cask` o desinstalar), y los dejan fuera. Los que se instalan
  con un `.pkg` o un instalador propio (piden la contraseña de
  administrador) se dejan fuera y se nombran cuando la ejecución no puede
  contestar a sudo: el agente pone `OMEGA_SUDO` (`yes` con `--sudo`, `ask`
  con alguien delante, `no` si nadie) en el entorno de las órdenes. Pruebas
  con un Caskroom falso en sh, bash y zsh.
- [x] **D10. Mole pide sudo por `/dev/tty` sin terminal** («/dev/tty:
  Device not configured», 22_144919:355, 23_080718:372). Debe: lanzarlo
  sin TTY en modo no interactivo, o con el ayudante de sudo de omega.
  Ahora: los pasos de Mole corren con `</dev/null`, se filtran las líneas
  «/dev/tty: Device not configured» y se conserva el código de Mole.
- [x] **D11. `mo purge` sin selección:** «No items selected», y el agente
  concluye que «no hay artefactos», mientras `mo clean --dry-run` dice
  «Build artifacts · 883 MB · mo purge» (23_081220:155, 23_082519:21).
  Debe: un paso `mole-purge` con la opción no interactiva que seleccione,
  y una pista de que «No items selected» no significa «nada que limpiar».
  Ahora: pasos `mole-purge-preview` (`mo purge --dry-run`, solo lectura) y
  `mole-purge` (`mo purge --yes`, artefactos de proyectos sin cambios en 7
  días; «confirmar siempre» por A4), y `config_hints` explica que «No items
  selected» no quiere decir «nada que limpiar».
- [x] **D12. La vista previa de topgrade falla en `containers` con Docker
  apagado** (23_080707:168), porque el `pre_command` que lo arranca solo se
  simula. Debe: en vista previa, saltar `containers` si `docker info`
  falla.
  Ahora: con `docker` instalado y `docker info` fallando, la vista previa
  añade `--disable containers` y dice por qué.
- [x] **D13. `mac_inventory` agota 90 s en `sfltool dumpbtm` y sale con
  0** (6 ejecuciones; en el otro Mac tarda 6 s). Los ítems de inicio
  nunca se revisan y el elemento roto «Alfred 5» se repite cada día.
  Debe: menos tiempo de espera, un estado «no disponible» visible, no
  repetir el intento en la misma sesión, y otra fuente (plists de BTM,
  System Events).
  Ahora: `sfltool dumpbtm` espera 20 s, un fallo se recuerda 6 horas y no
  se reintenta, y los ítems de inicio salen de System Events cuando falla
  (incluidos los rotos, cuya app ya no está). El informe dice que no se
  reintente.
- [x] **D14. `mac_inventory`:** `kMDItemLastUsedDate` es nulo para todas
  las apps (22_150714) y la comprobación de Rosetta del agente
  (`arch -x86_64 /usr/bin/true`) no mide nada. Debe: otra fuente de último
  uso y `lipo -archs` sobre el ejecutable para «solo Intel».
  Ahora: sin fecha de Spotlight, la fecha es la del último cambio de los
  ficheros propios de la app (preferencias, contenedor, estado guardado) y
  se dice que es una estimación; «solo Intel» ya se miraba con la
  arquitectura del ejecutable.
- [x] **D15. `mac_change` no dice si funcionó.** `osascript … >/dev/null
  2>&1 || { open …; }` no imprime qué rama se ejecutó, y el resumen afirmó
  «Shottr ya no se abre al iniciar sesión» sin comprobarlo (19_112126:15).
  Debe: informar de la rama y volver a leer el estado.
  Ahora: la orden de quitar un ítem de inicio comprueba después que ya no
  está y dice qué rama se ejecutó; si solo se abrió el panel, no se
  registra como hecho y el modelo debe darlo como pendiente.
- [x] **D16. `dev-artifacts`:**
  - escanea `~/Dev` y `~/dev` como dos raíces en un disco que no distingue
    mayúsculas, y cuenta todo dos veces (22_150146:27);
  - `-mtime +60` sobre la carpeta no indica abandono;
  - sin resultados no imprime nada (el agente adivina);
  - discrepa de Mole: 0 frente a 883 MB.

  Debe: deduplicar raíces por inodo, usar la fecha del último fichero del
  proyecto e imprimir «0 carpetas».
  Ahora: las raíces se deduplican por inodo, un proyecto está abandonado
  solo si ningún fichero cambió en 60 días (sin contar carpetas de
  compilación ni `.git`), y sin resultados imprime «0 stale build folders
  found».
- [x] **D17. `read_file` de un fichero que no existe sale con 0**
  (19_190418:13-22). Debe: error.
  Ahora: un fallo de `read_file` se registra como `[read_file FAILED]
  motivo`.

### E. Lo que el agente dice y no es verdad

- [x] **E1. El espacio libre sale de `df -h /`,** que en macOS es el
  volumen de sistema sellado (`systemInfoCommand`, `tools.go:239`). Cada
  ejecución da una cifra distinta (13, 17, 28, 29, 32, 36, 40 GB) y mezcla
  GB y GiB, y el agente se contradice («no tiene sentido», 19_111524:302).
  Debe: `system_info` da el libre del contenedor APFS
  (`/System/Volumes/Data`, más lo purgable) en una sola unidad, y el
  prompt dice que se use esa cifra antes y después.
  Ahora: `system_info` mide el volumen de datos (`/System/Volumes/Data`) y
  da una línea «FREE SPACE: N GB free of M GB (decimal GB)» que el prompt
  manda usar antes y después; el registro da la misma cifra en la cabecera.
- [x] **E2. Datos de notas o de ejecuciones anteriores presentados como
  medidos ahora:**
  - «`_wt` de Deudafix ~53 GB», «datos Postgres», «la imagen
    php74-apcu-10400 sigue ahí» con `Images 0` en la misma ejecución
    (23_082439:20 y :78);
  - «mo clean liberó 5,46 GB en la última pasada» (era otra);
  - «topgrade actualizó 47 paquetes» (falló; fue `brew`);
  - «usa `omega --sudo`» justo después de que sudo funcionara.

  Debe:
  - las notas llevan fecha y ejecución, y el prompt las presenta como «a
    comprobar»;
  - `run_history` da los pasos con su código de salida, para que el modelo
    no confunda topgrade con brew;
  - una nota caduca cuando lo medido la contradice.
  Ahora: el prompt dice que lo que dice una nota o una ejecución anterior
  es una pista que se mide de nuevo o se cita con su fecha; `run_history`
  da los pasos que fallaron con su código, y los pendientes (G1) se
  resuelven cuando la medida los desmiente.
- [x] **E3. El resumen afirma cosas que no se hicieron o no se midieron:**
  - «comprobado con comandos de solo lectura» sin comandos (19_185015:36);
  - «Go build ~444 MB» con el `du` fallido;
  - «go clean liberó 0,4 GB» con la carpeta inexistente;
  - «brew cleanup: descargas eliminadas» con «Pruned 0 symbolic links»;
  - «sin Xcode» con `/Applications/Xcode.app` presente (era
    `xcode-select` apuntando a CLT);
  - «Contenedores: OK» con el pull de `omega:ci` fallido;
  - el cambio de ControlMaster en «Hecho» sin escritura.

  Debe: el prompt exige que cada cifra o «hecho» del resumen salga de una
  salida de esta sesión, y `dev_inventory` informa de `xcode-select -p`
  frente a Xcode instalado.
  Ahora: el prompt exige que cada cifra y cada «hecho» salga de una salida
  de esta sesión, y `system_info` dice `xcode-select -p` y si Xcode.app
  está instalado.
- [x] **E4. Contradicciones dentro del mismo resumen.** Algunos ejemplos:
  - «Ejecuto lo que sí elegiste» y luego «no he cambiado nada»;
  - «Análisis hecho, sin cambios» antes de aplicar cuatro acciones;
  - pendientes repetidos dos veces (19_185353:31 y :61);
  - «Pendiente de tu parte: macOS no tiene novedades»;
  - mensajes en inglés en ejecuciones en español (19_184355:63,
    19_190543:29).

  Debe: un solo bloque final con «Hecho», «Pendiente para ti» y «Avisos»,
  en el idioma del usuario.
  Ahora: el prompt pide un solo bloque final (hecho, pendiente para el
  usuario, avisos), sin resúmenes intermedios, sin repetir, en el idioma
  del usuario.
- [x] **E5. Recuentos que no cuadran.**
  - `ls -d wt-*` (con el alias a eza) da 206 y `git worktree list`
    da 37.
  - El glob `wt-*` coge 170 ficheros `.pid`, `.err` y `.json`, con unas
    380 líneas de `fatal: … Not a directory`.
  - «1638 commits sin subir» se repite como hecho y probablemente es un
    `@{u}` mal resuelto.

  Debe: un inventario de worktrees en `dev_inventory`, o en `omega git`,
  que dé por cada uno si tiene cambios, commits sin subir, si se puede
  podar y su tamaño, calculado igual siempre (ver F3).
  Ahora: la herramienta `machine_facts` (sección `worktrees`) recorre los
  repositorios de las carpetas de proyectos (sin contar dos veces `~/Dev` y
  `~/dev`), y da por worktree cambios sin guardar, commits sin subir solo
  si tiene upstream, si se puede podar, su tamaño y la orden para quitar
  los limpios.

### F. Trabajo repetido y coste

- [x] **F1. Los mismos escaneos lentos en cada ejecución.**
  - `mo clean --dry-run` tarda entre 34 s y 1 min 23 s, a veces seguido
    del real.
  - `disk_usage` de `~` tarda entre 55 s y 1 min 47 s, varias veces por
    ejecución.
  - El bucle de 37 worktrees tarda entre 13 y 19 s.
  - `mac_inventory all` tarda 1 min 30 s.
  - `mo clean` se ejecutó cuatro veces en 25 minutos.

  Debe:
  - caché de resultados durante la sesión, y del día para los de solo
    lectura;
  - no repetir `mo clean` si se ejecutó hace menos de N horas;
  - no hacer la vista previa si va a seguir la ejecución real.
  Ahora: los resultados de pasos de solo lectura y `disk_usage` se guardan
  15 minutos en la sesión y se devuelven sin repetir, y cualquier cambio
  los borra; `mole-clean` y `mole-purge` no se repiten en 6 horas salvo
  `again=true`; el prompt pide no hacer la vista previa justo antes del
  real.
- [x] **F2. Llamadas repetidas.** `run_history` se llama 4 o 5 veces
  seguidas, `docker-df` dos veces con la misma salida, y la misma página
  de `ssh_config` y las de fin de vida de Python, Node, PHP y Ruby se
  vuelven a buscar y leer en ejecuciones separadas por 2 s. Debe:
  `run_history` devuelve las últimas N ejecuciones con sus pendientes, y
  `fetch_url` y `web_search` guardan caché un día.
  Ahora: `run_history` sin argumentos da también, de las 3 últimas
  ejecuciones, la cabecera, lo que falló o se rechazó y el final;
  `fetch_url` y `web_search` guardan su resultado un día en
  `~/.local/state/omega/web-cache`.
- [x] **F3. Comandos improvisados para lo mismo en cada ejecución.** El
  agente escribe a mano:
  - el bucle de estado de worktrees;
  - los `du` de cachés con rutas que no existen (`~/.config/mise`,
    go-build, uv, pip, pnpm…);
  - los `cat` de configuraciones que no existen;
  - `zsh -ic` (que carga starship, eza y zle en un terminal «dumb»);
  - `head -0`, que no vale en macOS.

  Debe: herramientas con salida estable para cada caso:
  - estado de worktrees;
  - rutas reales de caché (`go env`, `uv cache dir`, `npm config get
    cache`, `brew --cache`);
  - qué ficheros de configuración conocidos existen;
  - auditoría de PATH (entradas que no existen, variables vacías como
    `/pkg/env/global/bin`).
  Ahora: `machine_facts` da worktrees, rutas reales de caché (`go env`, `uv
  cache dir`, `npm config get cache`, `brew --cache`…), qué ficheros de
  configuración conocidos existen y cuáles no, y las entradas de PATH que
  no existen o salen de una variable vacía. El prompt manda usarla.
- [x] **F4. Sonnet para trabajos triviales.** 19_112246 y 19_185256
  arrancan con Sonnet para tres vistas previas ($0,10 cada una, 10 veces
  Haiku). Debe: Haiku para las tareas de vista previa o revisión de
  configuración, salvo que el usuario elija otro.
  Ahora: `fix` arranca con el modelo rápido y sube solo ante un fallo real
  (D3), como el resto de tareas.
- [x] **F5. Comprobar el precio de `claude-haiku-5-5`** en
  `internal/llm/pricing.go:27` (0,10/0,50 por millón, diez veces menos que
  `claude-haiku-4-5`). Si está mal, todos los costes de Haiku de estos
  registros están infravalorados.
  Ahora: el precio (0,10/0,50) es el de la tarifa hasta 100.000 tokens de
  entrada; por encima todo cuesta cinco veces más y omega no lo sabía.
  `Price` tiene ahora un tramo por tamaño del prompt y se aplica también
  con el precio del catálogo en vivo.

### G. Pendientes que se repiten en cada ejecución

- [x] **G1. Recordatorios con estado.** Estos avisos se repiten en cada
  ejecución y gastan tokens y atención:
  - Time Machine sin destino (más de 30 ejecuciones);
  - worktrees de doctorSIM (más de 15);
  - Docker prune (9);
  - «usa `omega --sudo`»;
  - el ítem de inicio roto «Alfred 5»;
  - el inicio de sesión de Cursor Agent;
  - Acceso total al disco;
  - el conflicto de ⌥Space;
  - `icu4c@78` deprecado;
  - ControlMaster.

  Debe:
  - un registro de pendientes con estado (nuevo, visto, pospuesto hasta,
    descartado) que el usuario puede marcar;
  - el resumen da solo lo nuevo o lo que cambió y una línea «N pendientes
    conocidos (`omega pending`)»;
  - un pendiente se cierra solo cuando la medida lo desmiente (la imagen
    de Docker que ya no existe).
  Ahora: `internal/pending` guarda los pendientes con estado (abierto,
  pospuesto hasta, descartado, hecho); la herramienta `pending` los anota y
  resuelve; cada ejecución los recibe en el prompt (los descartados solo
  por id, para no mencionarlos); `omega pending`, `pending dismiss|snooze
  <id> 2w|reopen|done` los gestionan. El resumen da en entero solo lo nuevo
  o cambiado.
- [x] **G2. Automatizar con aprobación lo que hoy se deja «a mano»:**
  - ControlMaster en `~/.ssh/config`, con copia; el agente no pudo
    aplicarlo en 3 ejecuciones;
  - quitar un ítem de inicio de BTM;
  - `git worktree remove` por worktree limpio, con su tarjeta;
  - el enlace a Ajustes de Time Machine y de Acceso total al disco, una
    vez.
  Ahora: el prompt pide ofrecer con tarjeta lo que antes se dejaba a mano:
  ajustes de `~/.ssh/config` con `write_file`, un ítem de inicio con
  `mac_change`, cada worktree limpio como opción con la orden de
  `machine_facts`; en desatendido los enlaces a Ajustes van al resumen (B6)
  y quedan como pendientes.
- [x] **G3. La imagen de CI de omega (`omega:ci`)** falla en cada `docker
  pull` de topgrade. Debe: `ignored_containers = ["omega:ci*"]` en el
  drop-in de omega.
  Ahora: el asistente de configuración de topgrade (`omega assist
  topgrade`, `internal/assist/maintenance.go`) tiene la comprobación
  `topgrade.omega-ci`, que añade `omega:ci*` a `[containers]
  ignored_containers` en el fichero donde está la lista que cuenta (el
  principal gana a los de `topgrade.d`), conservando las entradas del
  usuario y con copia. Tiene además `topgrade.pre-sudo`, que apaga
  `pre_sudo` y `sudo_loop` donde estén. `config_hints` apunta a las dos.
- [x] **G4. El drop-in de omega deja `system` (`softwareupdate --install
  --all`) activo en topgrade.** Puede reiniciar el Mac; el otro Mac lo
  desactivó a mano por eso. Debe: desactivarlo en el drop-in y dejar
  macOS al paso `macos-check`.
  Ahora: los pasos `topgrade` y `topgrade-preview` pasan siempre `--disable
  system` en macOS, sea cual sea el drop-in; las actualizaciones de macOS
  quedan en `macos-check`.
- [x] **G5. Las copias `.omega-bak-*` se acumulan** (6 de `omega.toml` en
  `~/.config/topgrade.d`). Debe: conservar las N últimas.
  Ahora: después de cada `write_file` que deja copia se conservan las 3 más
  recientes de ese fichero (`pruneBackups`).
- [x] **G6. Mole borró la caché del modelo en el dispositivo de Chrome
  (4,27 GB)**, que Chrome vuelve a descargar. La vista previa decía
  1,70 GB y el real fueron 6,90 GB (22_143904:164 y :886). Debe: excluirla
  y avisar cuando el real difiere mucho de la vista previa.
  Ahora: el asistente de Mole (`omega assist mole`) tiene la comprobación
  `mole.chrome-model`: añade `$HOME/Library/Application
  Support/Google/Chrome/OptGuideOnDeviceModel/*` a
  `~/.config/mole/whitelist`. Si el fichero no existe, empieza por la lista
  por defecto de Mole, leída del `lib/core/base.sh` instalado (un fichero
  propio la sustituye), y si no la encuentra no escribe nada.
  `config_hints` pide comparar cuando la limpieza real libera mucho más que
  la vista previa.

### H. Privacidad

- [x] **H1. Configuración sensible enviada entera al modelo y al
  registro.** Salieron:
  - `~/.ssh/config` con nombres de host, IPs de producción, usuarios y
    rutas de claves (23_082036:8-58), leído dos veces;
  - `git config --global --list` con `user.email`, los ayudantes de
    credenciales y las rutas de repos (22_150034, 23_082106, 23_082108).

  Debe: resumir o enmascarar esos ficheros (hosts e IPs no, alias sí;
  correos enmascarados) o leer solo las claves que hacen falta.
  Ahora: la salida de comandos y `read_file` sobre `~/.ssh/config` oculta
  `HostName`, `User`, `IdentityFile`, `Proxy*` y los reenvíos, y la de `git
  config`/`.gitconfig` las direcciones de correo.
- [x] **H2. La máscara de secretos tapa texto normal.** En 19_105805 la
  línea `…/get-account-password-policy.rst: Failed to create dir…` sale
  como `…password-policy.rst: [redacted] to create dir`. La regla de
  `password: valor` coge un nombre de fichero. Debe: exigir separador de
  clave real (inicio de línea, comillas o espacio) y añadir una prueba con
  ese caso.
  Ahora: una clave tiene que empezar la línea o ir tras un espacio,
  comilla, `{`, `,`, `(` o `[`, y un nombre que acaba en extensión seguido
  de `:` no se toma por clave (`TestScrubLeavesFileNamesWithSecretWords`).

## Cómo comprobarlo

- A1, A3, A4, B1, B4, C2, C3, D1 y D2 tienen prueba unitaria posible en
  `internal/agent` y `internal/safety`.
- C5 se comprueba con dos herramientas en paralelo y un registro.
- A6 y D8 necesitan un `Exec` falso que devuelva «No space left» o el
  fallo de sudo.
- Repetir `omega full --unattended` en los dos Macs y comparar los
  registros nuevos con estos: ningún «elegiste» sin pregunta registrada,
  ningún rechazo sin línea, la misma cifra de espacio libre en todas las
  ejecuciones y ningún salto a Sonnet por un aviso.

Estado: todo lo de arriba está hecho y tiene prueba automática (`go test
./...`, también compilado para Linux y Windows y la app con `-tags gui`).
Comprobado en este Mac con una ejecución real de solo lectura
(`omega health --dry-run --no-tui`, $0,008): cabecera completa, bloques
por llamada en paralelo, el rechazo con su motivo, la tarjeta con «chosen
by nobody», la cifra de espacio libre única y el recuento final. Los pasos
de topgrade, brew y Mole se probaron con programas falsos y, en este Mac,
con un `topgrade --dry-run` real, los recibos de Homebrew y la lista por
defecto de Mole instalada. Repetir `omega full --unattended` en los dos
Macs queda como comprobación de uso, no como cambio pendiente.

---

# TODO: tercera revisión de omega (seguridad de la CLI y de la app)

Revisión del 14 de noviembre de 2026 sobre `main` en `f737762` (v0.26.1),
solo de seguridad: qué puede hacer omega, o el modelo a través de omega, sin
que el usuario lo haya aprobado; qué secreto puede salir; y qué puede hacer
con omega otro programa, una página web o una carpeta que no es de fiar.

Los criterios son los de la revisión anterior (más abajo): un P0 ejecuta algo
sin la aprobación que omega promete, filtra un secreto o borra datos en un
uso normal. Cada P0 dice qué pasaba y qué pasa ahora, y está marcado porque
el cambio está hecho y tiene una prueba; lo que no la tiene lo dice.

## P0

### A. Comandos que se ejecutan sin la aprobación prometida

- [x] **A1. Un comentario falso esconde un comando.**
  `ls \ # ; rm -rf ~/Documents` se clasificaba como solo lectura: para el
  shell `\ ` es un espacio dentro de una palabra y `#` no empieza un
  comentario; para omega sí. Lo mismo con un retorno de carro delante de `#`.
  Ahora: `#` solo es comentario donde lo es para el shell.
- [x] **A2. Opciones largas abreviadas escriben ficheros.**
  `sort --out=$HOME/.zshenv` (abreviatura de `--output`) era de solo lectura,
  y ese fichero se ejecuta con el siguiente comando. También
  `sort --compress-prog`, `file --compil`, `date --se=…`, `pip list --log`.
  Ahora: una opción larga cuenta también por su abreviatura.
- [x] **A3. awk y jq leen el entorno o ejecutan código.**
  `awk 'BEGIN{f="sys" "tem"; @f("…")}'` (gawk), `@include`, `ENVIRON`,
  `ARGV`, y `jq -n 'env[]|explode'` eran de solo lectura.
  Ahora: piden aprobación.
- [x] **A4. La tarea programada era el agente con todo aprobado.**
  `omega schedule install` creaba `omega <tarea> --yes --no-tui`, que con una
  clave guardada arranca el modelo en modo automático, cada semana y sin
  nadie delante, aunque el mensaje de instalación describía los pasos
  integrados. Ahora: una ejecución programada son los pasos integrados y
  nunca el modelo, también en las tareas instaladas por una versión anterior
  (lo decide `OMEGA_SCHEDULED`, no solo el `--no-ai` que se añade).
- [x] **A5. `steps.d` no pasaba por la política de seguridad.**
  Un fichero ahí podía sustituir un paso de omega (`id = "disk"`) y ejecutar
  comandos de la lista de «nunca» (`rm -rf --no-preserve-root /`,
  `curl … | sh`) sin preguntar; los ganchos, igual; y se leía aunque
  cualquier usuario del equipo pudiera escribirlo.
  Ahora: un comando bloqueado se deja fuera y se dice, un paso propio no
  puede llevar el id de uno de omega, y no se lee un fichero (o una carpeta
  `steps.d`) que otro usuario pueda escribir o que sea de otro usuario.
  El riesgo declarado en el fichero (`risk = …`) se sigue respetando: es la
  configuración del propio usuario.
- [x] **A6. En modo automático, un comando podía cambiar lo que omega hará
  después.** Con `--yes` o tras «aprobar todo» se ejecutaban sin tarjeta
  `omega config set approval never`, un `sed` sobre `config.toml` (por
  ejemplo, para cambiar `base_url`, que es a donde se envía la clave),
  `echo … >> ~/.zshenv`, `cp x ~/Library/LaunchAgents/…`.
  `write_file` ya preguntaba siempre por esos ficheros; `run_command` no.
  Ahora: un comando que nombra un fichero de arranque del shell, un
  elemento de inicio, una tarea programada, la configuración, las notas o
  el estado de omega, o que ejecuta `omega`, se pregunta siempre, y en una
  ejecución desatendida se salta. Límite: mira el texto del comando; una
  ruta construida con variables (`d=~/.config/omeg; … ${d}a/…`) no se ve.
- [x] **A7. La tarjeta podía no mostrar lo que se iba a ejecutar.**
  Un comando con una secuencia de escape, un retorno de carro o una marca
  Unicode de derecha a izquierda se lee distinto de lo que ejecuta el shell.
  Ahora: `run_command` rechaza un comando con caracteres de control o
  invisibles, y lo que la interfaz muestra de cualquier herramienta los
  escribe como `<U+202E>`. Las secuencias OSC (título, portapapeles,
  enlaces) se quitan de la salida de los comandos.
- [x] **A8. Una carpeta con su propio `.git/config` ejecutaba un programa.**
  `omega git status`, `omega git clean`, `omega disk artifacts` y cualquier
  `git status` del agente (solo lectura) ejecutan `core.fsmonitor` del
  repositorio. Un `git clone` no trae ese fichero; una carpeta descomprimida
  o copiada sí. Ahora: git se llama siempre con `core.fsmonitor=false`.
  Queda por cerrar: los filtros `clean`/`smudge` y `diff.external` de un
  repositorio así (ver P1).
- [x] **A9. La lista de «nunca» se saltaba con comillas.**
  `dd of="/dev/disk0"`, `csrutil 'disable'`, `tmutil 'delete'`,
  `curl … | /bin/sh`, `| env sh`, `| sudo -E bash` solo pedían aprobación,
  así que con `--yes` se ejecutaban. Ahora: bloqueados. `omega mac undo`
  desde el agente ejecutaba el comando del diario sin clasificarlo: ahora
  pasa por la misma política.

### B. Secretos

- [x] **B1. La contraseña de sudo se podía leer.** El ayudante que responde
  a sudo está en el entorno de todos los comandos; ejecutarlo a mano
  devolvía la contraseña, y un `| rev` evitaba la máscara
  (`find "$TMPDIR" -name 'askp*' -exec {} x \; | rev`). Sin contraseña
  guardada, hacía aparecer la tarjeta de contraseña de omega para un comando
  que no era sudo. Ahora: omega solo responde cuando quien pregunta es un
  proceso iniciado por sudo (un proceso `sudo` que corre como root); a
  cualquier otro no le contesta ni pregunta al usuario.
- [x] **B2. Ficheros de credenciales legibles con comandos de solo
  lectura.** `grep -r '' ~/.config/gh`, `~/.docker`, `~/.claude`;
  `grep -r '' ~/.s*`; `cat ~/.netr[!x]`, `~/.config/gh/**/hosts.yml`;
  `cat ~/.ss${Z}h/id_r${Z}sa` (variable sin definir); `cat $KUBECONFIG`,
  `$GOOGLE_APPLICATION_CREDENTIALS`, `$DOCKER_CONFIG/config.json`; la copia
  `.omega-bak-…` que `write_file` deja de un fichero secreto; y ficheros
  que no estaban en la lista (claves de age/sops, `.s3cfg`, doctl, Vercel,
  Fly, Firebase, tokens de servidores MCP, `~/.zsh_sessions`).
  Ahora: bloqueados.
- [x] **B3. Las claves de los ficheros de arranque del shell, a trozos.**
  `cut -c 27-40 ~/.zshenv`, `grep -o . ~/.zshrc`, `tail -c 20 ~/.zshenv`
  eran de solo lectura y ninguna máscara reconoce una clave partida.
  Ahora: esos ficheros solo se leen sin preguntar con programas que
  imprimen líneas enteras (`cat`, `grep`, `head -n`…), donde la máscara
  funciona.
- [x] **B4. Las credenciales del entorno, a trozos.**
  `printenv 'GH_TOK'EN | cut -c1-10`, `env | grep --only .`,
  `env | head --byte=30`. Ahora: bloqueado o con aprobación.
- [x] **B5. La máscara de la salida solo conocía `NOMBRE=valor`.**
  No ocultaba `password = "…"`, `oauth_token: …`, `"access_token": "…"`,
  `Authorization: Bearer …`, ni un token reconocible por su forma
  (`ghp_…`, `github_pat_…`, `sk-…`, `xoxb-…`, `AKIA…`…), ni un bloque
  `BEGIN … PRIVATE KEY`. Ahora sí.

### C. Lo que sale del equipo y lo que queda para después

- [x] **C1. `fetch_url` enviaba lo que el modelo quisiera, sin preguntar.**
  En cualquier modo, también en solo lectura: tras leer un fichero, una
  página podía pedirle `https://sitio/?d=<lo leído>`. Ahora: sin preguntar
  solo se leen la documentación oficial que omega conoce, los resultados de
  su propia búsqueda en esa sesión y los sitios que el usuario nombra;
  cualquier otra dirección pide aprobación antes incluso de resolver el
  nombre, y en solo lectura o sin nadie delante no se pide: se rechaza.
- [x] **C2. `remember` dejaba instrucciones para todas las ejecuciones
  futuras.** Sin aprobación, sin límite, y las notas se añadían a las
  instrucciones del modelo tal cual. Ahora: una nota escrita después de
  leer la web pide aprobación siempre (también con `--yes`) y enseña el
  texto; es una línea de 400 caracteres como mucho, sin caracteres de
  control; las notas se presentan al modelo como datos que no puede
  obedecer; y el fichero es solo del usuario.

### D. App de escritorio

- [x] **D1. Un enlace podía llevar la ventana entera a otra página.**
  El menú del sistema sobre un enlace («Abrir enlace»), un clic central o
  un fichero soltado en la ventana cargan la página dentro de la app, donde
  puede hacerse pasar por omega y, según el sistema, llamar a sus
  funciones. Ahora: esos gestos no hacen nada sobre un enlace, y las
  funciones de la app rechazan una llamada cuyo origen es otra página.
  Límite: en macOS el webview no dice el origen de la llamada (comprobado en
  la ventana nativa), así que ahí la protección es que la ventana no navega.
- [x] **D2. Un enlace decía una dirección y abría otra.** Ahora el enlace
  muestra el sitio al que va cuando su texto no es la dirección.
- [x] **D3. «Exportar» sobrescribía cualquier fichero del usuario** sin
  preguntar (`~/.ssh/id_ed25519` incluido). Ahora solo escribe un fichero
  nuevo o sustituye una exportación anterior.

### E. Otros

- [x] **E1. `omega uninstall "Google Chrome"` seleccionaba los datos de
  Chrome Canary** (y los de cualquier app cuyo identificador empieza por el
  de otra), y una app con identificador `com` seleccionaba todo `com.*`.
- [x] **E2. `omega audit secrets` enseñaba parte del secreto** (la segunda
  mitad de un token de Slack, la contraseña de `curl -u`).
- [x] **E3. Menores:** el paso `temp-files` de Windows vaciaba la carpeta
  actual si `TEMP` no estaba definida; tres instaladores de `omega dev
  install` usaban `curl` sin `-f` ni `--proto '=https'`; `skills install`
  dependía de la configuración de git para rechazar `ext::`.

## Sin prueba automática

- B1 en Linux: la comprobación lee `/proc` y solo se ha probado su lectura
  con datos de ejemplo; el camino completo con un sudo que pide contraseña
  está probado en macOS. Con `/proc` montado con `hidepid`, omega no podrá
  reconocer a sudo y no responderá.
- D1 y D2: comprobado en la ventana nativa de macOS que las llamadas de la
  propia app siguen pasando; el rechazo de una página ajena y el menú sobre
  un enlace no se han ejecutado en una ventana.
- E3 (Windows) solo se compila.

## P1 pendientes de esta revisión

- [x] **Modo automático y código opaco.** Con `--yes`, `… | base64 -d | sh`,
  `sh fichero`, `python3 -c …` o `docker run -v ~:/h …` solo son
  «peligrosos» y se ejecutan; lo mismo `gh auth token`, `gcloud auth
  print-access-token`, `security -i`. Hace falta una segunda clase de
  «preguntar siempre». La clave del proveedor se retira por nombre pero su
  valor no está registrado en la máscara.
  Ahora: «confirmar siempre» (`internal/safety/ask.go`) cubre lo que se
  entuba a un intérprete, `sh`/`bash` con un fichero y `source`,
  `python`/`node`/`perl`/`ruby`/`php` con `-c`/`-e` o un fichero,
  `osascript` con «do shell script», `pwsh -EncodedCommand`, `docker
  run`/`create` que monta `/`, `~` o `$HOME` o es `--privileged`, y los que
  imprimen credenciales (`gh auth token`, `gcloud auth print-*-token`, `az
  … get-access-token`, `security -i`, `security find-*-password -w`). El
  valor de la clave del proveedor se registra en la máscara
  (`runner.Redact`) al crear el proveedor.
- [x] **sudo.** Un ayudante falso puesto con `SUDO_ASKPASS` bajo un sudo
  real aún puede recibir la contraseña; requiere un comando con `sudo`
  aprobado.
  Ahora: un comando que ejecuta sudo y además cambia el entorno (una
  asignación, `sudo VAR=…`, `export`, `eval`, `source`…) se bloquea, y el
  preámbulo del intérprete vuelve a poner `SUDO_ASKPASS` después de leer
  los ficheros de arranque, así que una línea de `~/.zshenv` no lo
  sustituye.
- [x] **Borrados del directorio personal no bloqueados:** `rm -rf ~usuario`,
  `rm -rf $PWD`, `rm -rf /Users/l*`, `mv ~ /tmp/x`, `rsync --delete … ~/`,
  `git -C ~ clean -fdx`, y herramientas de disco que no están en la lista
  (`mke2fs`, `sgdisk --zap-all`, `shred /dev/…`, `asr restore --erase`).
  Ahora: bloqueados `rm -rf ~usuario`, `$PWD`/`$OLDPWD`, los patrones que
  alcanzan una carpeta personal o la raíz (`/Users/l*`, `/home/*`), `mv ~
  …`, `rsync --del… … ~/`, `git -C ~ clean -f…` y `cd ~ && git clean -fdx`,
  y las herramientas `mke2fs`, `mkswap`, `sgdisk --zap-all/-Z/-o`, `shred
  /dev/…`, `asr … --erase`. La carpeta de otro usuario escrita entera (`rm
  -rf /home/bea`) sigue siendo «peligrosa» a propósito, como dicen las
  pruebas anteriores.
- [x] **«Preguntar siempre» no reconoce** `podman system reset`,
  `docker compose down --volumes=true`, `docker.exe …`, `docker service rm`.
  Ahora: reconoce `podman system reset`, `podman machine reset`, `docker
  service|secret|config|node rm`, `swarm leave`, `compose down
  --volumes=true` (no `=false`) y `docker.exe`.
- [x] **Windows:** `ipconfig /release`, `net use * /delete`, `net start`,
  una ruta UNC (`dir \\host\share` envía el hash NTLM) y familias enteras
  de verbos `Get-…` son de solo lectura; `Get-ChildItem ~\.ssh |
  Get-Content` lee las claves.
  Ahora: `ipconfig` solo es de solo lectura solo o con `/all` o
  `/displaydns`, `net use`/`net start`… sin más argumentos; una ruta UNC
  hace peligroso el comando; los `Get-` son una lista permitida
  (`Get-Credential`, `Get-Secret`, `Get-AzAccessToken` ya no),
  `-AsPlainText` cuenta como escritura; listar una carpeta de secretos
  hacia un lector (`Get-ChildItem ~\.ssh | Get-Content`, `ls ~/.ssh | xargs
  cat`) se bloquea.
- [x] **git en carpetas no fiables:** filtros `clean`/`smudge`,
  `diff.external` y `textconv` del `.git/config` del repositorio.
  Ahora: `runner.UntrustedGitConfig` reconoce un repositorio (o worktree)
  cuyo `.git/config` nombra un filtro `clean`/`smudge`/`process`, un
  `textconv` o un programa de diff o merge (Git LFS aparte), y `omega git`,
  `omega disk artifacts` y `machine_facts` no lo miran y lo dicen;
  `fsmonitor` ya estaba apagado. `diff.external` no se puede vaciar sin que
  `git diff` falle; omega no ejecuta diffs en esas carpetas.
- [x] **`write_file`:** la tarjeta de un fichero nuevo no enseña su
  contenido; faltan en «preguntar siempre» `~/.aws/config`
  (`credential_process`), `~/.kube/config`, ejecutables existentes.
  `~/.gitconfig` queda fuera a propósito.
  Ahora: la tarjeta de un fichero nuevo enseña su contenido (principio y
  final si es largo), y escribir `~/.aws/config`, cualquier cosa bajo
  `~/.kube/` o un fichero ejecutable que ya existe se pregunta siempre.
- [x] **`mac_policy` cambia `mac.toml` sin aprobación** y `mac_change
  disable:` es «normal».
  Ahora: `mac_policy` pide aprobación (en desatendido no se guarda nada) y
  `mac_change disable:` es «peligroso».
- [x] **`propose_actions` levanta el modo de solo lectura** con cualquier
  elección, y en una ejecución desatendida se eligen todas.
  Ahora: elegir en la tarjeta pasa a «preguntar» (no a automático aunque
  hubiera `--yes`), así que cada orden que cambia algo sigue pasando por su
  tarjeta; en desatendido ya no se toman todas (A1 de la cuarta revisión).
- [x] **`omega uninstall Code`** (sin app con ese nombre) ofrece los datos
  de Visual Studio Code.
  Ahora: si ninguna app se llama así pero una instalada usa ese nombre por
  dentro (el ejecutable o el nombre corto de su paquete, como `Code` en
  Visual Studio Code), se rechaza con el nombre de esa app.
- [x] **`fetch_url`:** rangos que el filtro no cubre (NAT64, 6to4,
  `198.18.0.0/15`); con un proxy solo queda la comprobación previa.
  Ahora: se rechazan también 198.18.0.0/15, los rangos de documentación y
  reservados, NAT64 (`64:ff9b::/96`, `64:ff9b:1::/48`), 6to4 (`2002::/16`)
  y Teredo. Con un proxy sigue sin poder mirarse la dirección final: queda
  la comprobación previa del nombre.
- [x] **Comandos de solo lectura que no terminan:** `mdfind -live`,
  `tail --foll`, `pmset -g pslog`.
  Ahora: no son de solo lectura `tail`/`journalctl`/`docker logs` con
  `--follow` abreviado, `mdfind -live`, `pmset -g
  pslog/rawlog/thermlog/assertionslog`, `log stream`, `fs_usage`,
  `powermetrics`, `tcpdump`, `iostat` con intervalo y sin `-c`, y `ping`
  sin `-c`.
- [x] **Ficheros y límites:** `config.toml` y `mac.toml` son legibles por
  otros usuarios; las respuestas del proveedor se leen sin límite; no hay
  tope de llamadas por turno; la unidad de systemd no escapa `%` ni `$`.
  Ahora: `config.toml` y `mac.toml` se escriben con permisos 0600; una
  respuesta de un proveedor compatible con OpenAI se lee hasta 32 MB; un
  turno ejecuta como mucho 16 llamadas y las demás se contestan sin
  ejecutar; la ruta de omega en la unidad de systemd escapa `%`, `$`, `\` y
  `"`.
- [x] **App:** `RunTool` no se puede cancelar; `Browser.OpenURL` acepta
  esquemas que no son http(s) si los pide un script de la página.
- [x] **Publicación:** fijar por commit las acciones `docker/*`, añadir
  atestación de procedencia a los archivos, hash SRI al reproductor de
  asciinema de la web; `omega net audit` imprime las credenciales de un
  proxy.
  Ahora: la app tiene `StopTool` y el botón de una herramienta en marcha la
  para (`cli.RunToolContext`: la orden recibe el contexto y la ventana queda
  libre al momento). Una llamada `Browser.OpenURL` que no es http(s) se
  rechaza en el mismo filtro que protege `/wails/runtime`, también enviada
  en trozos (`internal/gui/guard.go`, con prueba). En `release.yml` las
  acciones `docker/*` van fijadas por commit, y el trabajo `publish` firma
  la procedencia de los archivos (`actions/attest-build-provenance`,
  comprobable con `gh attestation verify`). La web carga el reproductor de
  asciinema con `integrity` (sha384) y `crossorigin`. `omega net audit`
  muestra un proxy como `http://***@host:3128`.

---

# Segunda revisión (13 de noviembre de 2026)

Revisión del 13 de noviembre de 2026 sobre `main` en `163b7eb` (v0.25.0),
después de corregir los 30 P0 de la primera (v0.25.0).

Un P0 es un defecto que, en un uso normal, borra datos del usuario, deja
ejecutar un comando sin la aprobación que omega promete, filtra un secreto,
o deja una función principal rota. Un P1 da un resultado equivocado, un
mensaje que engaña, un código de salida incorrecto o un cuelgue.

Cada punto dice qué pasaba y qué tiene que pasar. Se marca cuando el cambio
está hecho y tiene una prueba; lo que no la tiene lo dice.

## P0

### A. Pérdida de datos

- [x] **A1. La Papelera de omega sobrescribe lo que ya tenía** (regresión de
  la v0.25.0). El nombre en caso de choque era la hora al milisegundo: dos
  elementos con el mismo nombre en el mismo milisegundo acababan en uno.
  `omega uninstall` con 30 ficheros `com.foo.Bar.plist` dejaba 7 en la
  Papelera y perdía 23; `omega skills remove` con tres copias de una skill
  dejaba dos carpetas mezcladas.
  Debe: un nombre libre cada vez (`foo`, `foo 2`…), copiar solo cuando la
  Papelera está en otro volumen, y no borrar nunca una carpeta de la
  Papelera que no haya creado esa misma llamada.
- [x] **A2. `omega bootstrap <fichero>` ejecuta texto del fichero y cambia la
  configuración sin preguntar.** Un nombre de paquete
  `--help; touch ~/X` se ejecutaba tal cual, `approval = "never"` se
  restauraba desde el fichero, y siempre terminaba con «completado».
  Debe: validar los nombres, enseñar el plan y preguntar (o `--yes`),
  `--dry-run`, no restaurar nunca `approval` ni la URL del proveedor, copia
  de la configuración anterior, y error si algo falla.
- [x] **A3. Una pregunta libre con `--no-ai`, `--offline` o `--json` lanza el
  mantenimiento completo.** `omega --json qué ocupa mi disco` descartaba la
  pregunta y ejecutaba actualización, limpieza y análisis.
  Debe: error diciendo que una pregunta libre necesita la IA.
- [x] **A4. Con un `config.toml` ilegible, el modo sin IA usa los valores por
  defecto** e ignora `skip_steps`: ejecuta los pasos que el usuario había
  quitado. Lo mismo al caer a modo sin IA por un `--provider` mal escrito.
  Debe: devolver el error; caer a modo sin IA solo cuando falta la clave.
- [x] **A5. La tarea programada vacía la Papelera cada semana,** que es
  donde omega deja lo que quita para poder devolverlo.
  Debe: una ejecución programada no vacía la Papelera, y `schedule install`
  dice qué hace la tarea.

### B. Comandos que se ejecutan sin la aprobación prometida

- [x] **B1. Un comentario `#` esconde un comando.**
  `echo hi # "⏎rm -rf dir # "` se clasificaba como solo lectura.
  Debe: un `#` sin comillas al empezar una palabra llega hasta el fin de línea.
- [x] **B2. Las opciones se miran en el texto, pero el shell quita comillas y
  expande.** `find . "-delete"`, `find . ${TERM:+-delete}`,
  `find . -{delete,delete}`, `sort "-o" ~/.zshenv`, `rg '--pre' sh`,
  `git log "--output=…"` pasaban como solo lectura.
  Debe: mirar las palabras como las ve el shell; una expansión con operador
  o de llaves en un comando «de solo lectura» es peligrosa.
- [x] **B3. `<comando> <palabra> help` o `version` es «solo lectura»:**
  `rm foo help` borraba los dos ficheros.
  Debe: solo `--help`, `-h`, `--version`, o `help`/`version` como único
  argumento.
- [x] **B4. Prefijos de solo lectura con una cola que cambia cosas:**
  `git remote -v add evil URL`, `softwareupdate -l -i -a -R`,
  `mdutil -s -E /`. Debe: solo las opciones permitidas detrás.
- [x] **B5. PowerShell:** `Get-ChildItem | ForEach-Object Delete`,
  `Get-Process x | % Kill`, `dir | where { del $_ }`,
  `ls | sort { del $_ }`. Debe: peligroso.
- [x] **B6. `omega steps run <id> --yes` se salta «confirmar siempre»**
  (`docker volume prune -a -f` sin terminal).
  Debe: la misma regla que el modo sin IA; sin terminal se salta y sale con 1.
- [x] **B7. `approval = "all"` no se respeta en modo sin IA.**
  Debe: preguntar cada paso que no sea de solo lectura; sin terminal, saltarlo.
- [x] **B8. App: una imagen Markdown en el texto del modelo puede llamar a
  las funciones de la app sin un clic.** La CSP de la v0.25.0 dejaba cargar
  imágenes propias, y `/wails/runtime?…` es una dirección propia: con ella
  se podía aprobar un comando, pasar a modo automático o cambiar el
  proveedor.
  Debe: una imagen se muestra como su texto, la CSP no deja cargar ninguna
  por dirección, y las funciones de la app solo responden a POST.

### C. Secretos

- [x] **C1. La sustitución por `[redacted]` se salta con comandos de solo
  lectura:** `echo ${GITHUB_TOKEN:0:10} ${GITHUB_TOKEN:10}` o
  `printenv GITHUB_TOKEN | grep -o .`.
  Debe: un comando que nombra una variable secreta no se ejecuta.
- [x] **C2. Secretos que nunca se registran:** nombres como `STRIPE_KEY`,
  `DB_PASS`, `MYSQL_PWD`, URLs con contraseña (`postgres://u:pw@host`), y
  todo lo que solo exporta el fichero de arranque del shell (el caso normal
  en la app, que no hereda el entorno del terminal).
  Debe: registrarlos, y ocultar el valor de cualquier línea `NOMBRE=valor`
  con nombre de secreto.

## P1

### D. Seguridad

- [x] **D1. Ficheros de credenciales legibles sin preguntar:** historiales
  del shell, `~/.pgpass`, `~/.claude/.credentials.json`, `~/.codex/auth.json`,
  `~/.kube/*`, `~/.azure/*`, cookies del navegador, `/etc/shadow`…
- [x] **D2. El ayudante de sudo no estaba cubierto por el bloqueo** (ruta
  `omega-sudo-…/askpass`).
- [x] **D3. «Confirmar siempre» no reconocía** `docker --context x system
  prune`, `docker compose down -v`, `docker volume remove`, `podman`…
- [x] **D4. `write_file` promete aprobación siempre y no la exige** en modo
  automático; lee el fichero antes de pedirla (se cuelga con una tubería);
  y la vista previa corta en 40 líneas sin avisar.
- [x] **D5. `echo hi >&2file` crea un fichero sin preguntar.**
- [x] **D6. Comandos «de solo lectura» que no terminan** (`tail -f`,
  `top -l 0`, `cat /dev/zero`…) retienen la sesión 45 minutos.
- [x] **D7. Huecos en la lista de «nunca»:** la carpeta personal por ruta
  absoluta, `cd ~ && rm -rf *`, `diskutil apfs eraseVolume`, `newfs_*`,
  `Remove-Item -Path:C:\ -Recurse`…
- [x] **D8. La clave de API pasa por los argumentos de `security`** (visible
  en `ps`).
- [x] **D9. El clasificador pregunta por comandos corrientes de solo
  lectura** (`cd x && git status`, `awk`, `cut`, `git diff`, `lsof -i`,
  `docker logs`, `echo "a -> b"`…) y bloquea `fdesetup status`, `nvram -p`
  o `man mkfs`.

### E. Sesión y proveedores

- [x] **E1. Un proceso en segundo plano hace «fallar» el comando** tras 5 s.
- [x] **E2. En Windows, cancelar mata PowerShell y no a sus hijos.**
- [x] **E3. Una respuesta vacía del modelo deja el historial inválido**
  (Anthropic y OpenAI), y un resultado vacío de herramienta también.
- [x] **E4. No hay compactación del historial:** una sesión larga muere con
  «prompt too long».
- [x] **E5. OpenCode: cambiar de modelo entre familias sigue usando el
  formato equivocado.**
- [x] **E6. Dos sesiones en el mismo segundo se pisan el registro;** parar
  por presupuesto sale con 0; un tiempo de espera de 10 minutos se
  reintenta cuatro veces.

### F. CLI

- [x] **F1. Ctrl+C en una pregunta de `--no-tui` se cuelga** (elección,
  confirmación, contraseña).
- [x] **F2. Los ganchos (`hooks`) se ejecutan con `--dry-run`, nunca con
  `--no-ai`,** y sus fallos se pierden.
- [x] **F3. `omega full --json` mezcla texto y dos documentos JSON;**
  `dev --no-ai`, `mac --json`, `fix --json` anuncian opciones que fallan.
- [x] **F4. Un subcomando desconocido sale con 0** en `net`, `audit`, `git`,
  `disk`, `schedule`, `completion`, `auth`, `config`.
- [x] **F5. Opciones y argumentos que se ignoran:** `--format yaml`,
  `--effort bogus`, `--max-turns -3`, `net ports abc`, `disk large /noexiste`.
- [x] **F6. Pasos propios: un `steps.d/*.toml` con un error se descarta en
  silencio,** y una categoría mal escrita deja el paso invisible.
- [x] **F7. `config set` sustituye un `config.toml` enlazado por un fichero
  normal;** `config edit` falla con `EDITOR="code -w"`.
- [x] **F8. Textos de ayuda que no son verdad** (`--model`, `omega man`,
  `quick` dice «tu equipo está sano» sin haberlo mirado).
- [x] **F9. `quick` cuenta avisos como paquetes pendientes** e ignora
  `skip_steps`; el informe JSON da duraciones a cero y `steps_run: null`.
- [x] **F10. Las preguntas sí/no solo aceptan `y`:** `s`, `sí`, `oui`, `ja`
  eran un «no» silencioso.
- [x] **F11. `omega setup` sin terminal sale con 0 sin configurar nada;**
  `completion install` en Windows instala la de bash.
- [x] **F12. `omega steps --json` y `omega doctor --json`** (no existían).

### G. Ficheros y sistema

- [x] **G1. `disk artifacts` da por inactivos proyectos que se editan hoy**
  (no miraba carpetas profundas y paraba a las 5.000 entradas), y dice
  «✓ borrado» cuando el borrado falló.
- [x] **G2. `omega uninstall Mail` selecciona los datos de la app del
  sistema;** `uninstall Desktop`, los de Figma y GitHub Desktop.
- [x] **G3. `skills install --force` y `skills update` sustituyen una
  carpeta fuera de las ubicaciones de skills** (un enlace a un clon pierde
  su `.git`).
- [x] **G4. `omega mac undo` dice ✓ sin restaurar** (el comando terminaba en
  `; true`), y una restauración a medias no se puede terminar.
- [x] **G5. `schedule install` dice «instalado» cuando launchd rechazó la
  tarea;** el registro iba a `/tmp`; con Homebrew en Linux la ruta dejaba
  de existir tras actualizar.
- [x] **G6. `net flush` dice que vació la caché sin haberlo hecho;**
  `net ports` y `net kill` incluyen procesos UDP; `net audit` da por
  sospechoso el `/etc/hosts` de fábrica.
- [x] **G7. `audit secrets` no detecta claves `sk-proj-…`,** se para en una
  línea larga y enseña hasta 31 caracteres de la clave; `audit ssh` da
  falsos positivos con enlaces y rutas entre comillas.
- [x] **G8. `omega privacy` no lee ningún ajuste** («telemetría activa» era
  un texto fijo) y `--clean` dice ✓ cuando falla.
- [x] **G9. `git status` omite sin avisar los repos que tardan más de 3 s;**
  `git clean` ofrece ramas de un worktree y dice «borrado» sin borrar.
- [x] **G10. En Windows la batería sale como «0,0 % de capacidad».**
- [x] **G11. Código sin uso que borraría datos si se conectara**
  (archivos de Xcode, `vssadmin delete shadows /all`): eliminado.

### H. App de escritorio

- [x] **H1. Recargar la ventana (⌘R) deja la sesión huérfana:** la app
  parece libre, no deja empezar otra y no hay forma de parar la que sigue.
  Debe: sin «Recargar» en el menú, y una ventana que se vuelve a dibujar
  muestra la sesión y sigue.
- [x] **H2. Cerrar la ventana con una sesión en marcha la mata sin avisar.**
  Debe: preguntar.
- [x] **H3. Los errores llegan en inglés sin traducir** y el aviso de «no se
  pudo iniciar» siempre manda a configurar.
- [x] **H4. «Detener» durante el arranque no da señal de vida.**
- [x] **H5. Foco y lectores de pantalla:** tras responder una tarjeta el
  foco se pierde; una tarjeta nueva no se anuncia.
- [x] **H6. La CSP solo cubría imágenes.** Debe: `default-src 'self'`.
- [x] **H7. Menores:** el estado dice «Trabajando…» con un comando aún en
  curso; la salida recortada no lo dice; una contraseña escrita y no
  enviada se queda en pantalla.

### I. Publicación

- [x] **I1. La imagen de contenedor se publica aunque el release falle**
  (los trabajos `image` y `publish` no dependían uno del otro).
- [x] **I2. El trabajo `publish` no se puede relanzar** tras un fallo a
  medias (`gh release create` falla porque ya existe).
- [x] **I3. Todos los trabajos del release tenían permiso de escritura.**
- [x] **I4. La nota del cask decía que el `xattr` hace falta «una vez»:**
  hace falta tras cada `brew upgrade --cask`. La nota de
  `lcajigasm/homebrew-tap` la escribe su propio script y queda por cambiar
  allí.
- [x] **I5. La documentación prometía «avisos de escritorio»** que nada envía.

## Sin prueba automática

H1 a H7 se comprobaron en la app en modo servidor (recarga, foco, CSP) y la
CSP y el arranque de sesión también en la ventana nativa de macOS; el menú
y el aviso al cerrar no se han ejecutado. E2 y G10 (Windows) solo se
compilan. I1 a I3 se prueban al publicar.

## Fuera de esta ronda

- Traducir las descripciones de 14 comandos y la salida del modo sin IA,
  que siguen en inglés.
- Fijar las acciones de GitHub por commit en vez de por etiqueta, y
  comprobar la suma de `vhs`, `agg` y `ttyd` en `captures.yml`.
- Firmar y notarizar la app de macOS (quita el `xattr` de raíz).
- `approval = "never"` no se aplica en modo sin IA (los pasos peligrosos
  siguen pidiendo `--yes` o una respuesta). Es deliberado.
- `fetch_url` no pide aprobación para una dirección pública, y `remember`
  escribe notas sin aprobación: una página puede influir en sesiones
  futuras.
- Un enlace Markdown puede mostrar un texto y abrir otra dirección.
- `full --json` se rechaza; falta un documento agregado.
- Las herramientas de la app (`RunTool`) no se pueden cancelar.
