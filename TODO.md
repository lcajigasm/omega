# TODO: quinta revisión de omega (la CLI a fondo y sus interfaces)

Revisión del 24 de noviembre de 2026 sobre `main` en `0537a89` (v0.31.0).
Las cuatro revisiones anteriores (13, 19 y 23 de noviembre) tenían todos sus
puntos cerrados; su texto sigue en el historial de git
(`git show 0537a89:TODO.md`).

Esta vez no se parte de ejecuciones reales, sino de leer el código y probar
la CLI. La revisión se hizo en tres partes:

1. la superficie de órdenes, opciones, ayudas, errores y salidas;
2. el núcleo: agente, política de seguridad, proveedores, ejecución y
   estado;
3. la interfaz de terminal: TUI, tarjetas y salida de texto.

Se ejecutaron las órdenes que solo leen. Las que borran o cambian algo se
revisaron leyendo su código. Las afirmaciones sobre la política salen de
pruebas de sondeo reales contra `safety.Classify` y `safety.AlwaysConfirm`.

Criterios, los de siempre:

- **P0:** hace algo que el usuario no ha aprobado, borra datos o deja rota
  una función principal.
- **P1:** da un resultado equivocado, engaña, cuesta tiempo o dinero sin
  motivo, o impide revisar lo que pasó.
- **P2:** fricción o incoherencia.
- **P3:** pulido.

Cada punto dice qué pasa, dónde está en el código y qué tiene que pasar.

**Estado (24 de noviembre, segunda pasada):** todos los puntos están
implementados. Bajo cada uno, la línea **Hecho:** resume el cambio y, cuando
lo hay, lo que se dejó distinto de lo pedido y por qué. Las capturas de
`docs/assets` están regeneradas con los glifos nuevos salvo `sudo.png`, que
necesita root y `runuser` (Linux) y la regenera el flujo `captures.yml`.

---

## Hecho en la primera pasada

- [x] **Modo `omega custom`: tu rutina, sin IA.**
  - `omega custom --set` abre un editor a pantalla completa con dos zonas:
    arriba la rutina, numerada y en orden de ejecución; debajo, los pasos
    instalados que aún no están en ella, por categoría.
  - Teclas del editor:
    - espacio: añadir o quitar;
    - ⇧↑/⇧↓ o J/K: reordenar;
    - `c`: vaciar; `u`: deshacer;
    - enter: guardar; esc: salir (pregunta si hay cambios sin guardar).
  - Abajo se ve el comando, el riesgo y el motivo del paso bajo el cursor.
  - `omega custom --set id…` fija la rutina sin editor, para scripts.
  - `omega custom` la ejecuta aquí, paso a paso, con las reglas de
    aprobación de `omega steps run`, y termina con una tabla de resultados.
    Nunca llama a un modelo.
  - Otras opciones: `--show`, `--dry-run`, `-y` y `--clear`.
  - La rutina se guarda en `~/.config/omega/custom.toml`. Los pasos que
    faltan en una máquina se conservan para las demás que compartan el
    fichero.
  - Código: `internal/ui/routine.go`, `internal/cli/custom.go`,
    `internal/config/custom.go`.
  - Pruebas: `internal/ui/routine_test.go`, `internal/cli/custom_test.go`.
- [x] **Recomendación de la IA dentro del editor.**
  - Con `i` se pide al modelo configurado una rutina: pasos, orden, un motivo
    por paso y un resumen. Solo envía la lista de pasos instalados (id,
    categoría, riesgo, descripción), el sistema operativo y la rutina
    actual, y solo cuando se pulsa `i`.
  - La respuesta se valida: se descartan ids inventados y repetidos.
  - Se muestra frente a la rutina actual (lo que añade, lo que dejaría
    fuera). `a` la adopta, aunque sigue siendo editable. `esc` mantiene la
    rutina propia y conserva los motivos como notas ✦.
- [x] **Un aspecto común para las órdenes de una sola salida**
  (`internal/ui/report.go`):
  - título `Ω  omega <orden> · contexto` y secciones en el color de acento;
  - marcas ✓ ! ✗ · – › ▸ con color;
  - tablas alineadas por columnas visibles, no por bytes;
  - texto ajustado al ancho del terminal;
  - sin color al redirigir la salida, con NO_COLOR o con TERM=dumb.

  Ya lo usan:
  - `steps`: columnas alineadas entre categorías, recuento, una leyenda con
    los mismos glifos y una pista hacia `custom`;
  - `logs`: fecha, tarea, modelo, coste y estado de cada ejecución, leídos
    de la cabecera y del cierre del registro, en vez de «16464 B»;
  - `pending`;
  - `notes`;
  - `doctor`;
  - `models`;
  - `custom`.
- [x] **Riesgo en palabras cortas y exactas**: «solo lee», «cambia» y «pide
  permiso». Antes `steps` decía «peligroso» de `docker-start` y de cualquier
  caché.
- [x] **`omega pending`:**
  - el estado sale traducido (abierto, pospuesto hasta…, descartado, hecho);
  - el texto se ajusta al terminal, sin columnas fijas de 28 caracteres;
  - por defecto solo se ven los activos, y `--all` muestra el resto;
  - en `done|dismiss|snooze|reopen` basta con el principio del id, así que
    el ejemplo de la ayuda (`time-machine`) ya funciona;
  - los textos nuevos se recortan sin partir caracteres UTF-8, en el límite
    de palabra y con «…» (`pending.clip`). Antes era `s[:300]`.
- [x] **`omega notes`:**
  - las más recientes primero, agrupadas por día y ajustadas;
  - `--last N` (15 por defecto) y `--grep texto`;
  - `notes clear` pregunta antes de borrar y dice que lo hizo;
  - `remember` ya no guarda la fecha dos veces («2026-10-27: 2026-10-27
    full run») y comprueba los errores de escritura y de cierre, así que ya
    no dice «saved» con el disco lleno.
- [x] **`omega doctor`:**
  - cada comprobación lleva su marca (✓ ! ✗ ›) y la etiqueta en negrita;
  - los 82 pasos se ven en una rejilla de 3–4 columnas, no en 82 líneas;
  - el resumen del recorrido guiado ya no filtra líneas del texto a mano:
    llama al informe sin la lista de pasos.
- [x] **`omega models`:** la tabla se alinea sola. Antes la cabecera «entrada
  $/M» no cabía en `%9s` y se descuadraba.
- [x] **Menú de inicio:**
  - incluye `custom` y sus variantes (`--set`, `--show`, `--dry-run`);
  - al volver de una orden dice si terminó bien (✓) o con qué código salió
    (✗), antes de «Enter vuelve al menú».
- [x] **Arreglos de la TUI:**
  - glamour respeta el perfil de color del terminal (NO_COLOR, 256 colores)
    y no pasa de 110 columnas de prosa en pantallas anchas;
  - la barra de disco se lee sin color (━ usado, ─ libre);
  - el resumen final no deja un « · » colgando cuando no se conoce el coste,
    porque se construye por partes;
  - el coste se formatea igual en todas las pantallas (`ui.FormatCost`):
    cuatro decimales por debajo de $0,10 y dos por encima;
  - las zonas clicables de la tarjeta de confirmación se miden en columnas:
    en es/fr/pt se desplazaban con «sí» o «à»;
  - el borrado del filtro del picker corta por runas («ñ», «é»);
  - el menú de inicio y el piloto resuelven el color de fondo antes de
    arrancar Bubble Tea. La respuesta OSC 11 llegaba como Esc y podía cerrar
    el menú nada más abrirlo;
  - `truncate` mide columnas (`ansi.Truncate`): los caracteres anchos ya no
    desbordan el panel.
- [x] **Texto plano (`--no-tui`):**
  - un éxito de menos de 1 s también imprime `⎿ ✓`;
  - una herramienta denegada sin argumento ya no sale como «System()»;
  - el recorrido guiado ya no ofrece `[p] cambiar el plan`, que en texto no
    existe;
  - las duraciones usan el mismo formato que la TUI (`1m05s`).
- [x] `--debug` (no hace nada) y `--offline` (engaña: los pasos siguen
  usando la red) se ocultan de la ayuda; siguen funcionando.
- [x] El manual documenta `omega custom` (`docs/manual/tasks.md`,
  `docs/manual/reference.md`) junto con las opciones nuevas de `notes` y
  `pending`.

---

## P0

### A. Lo que se ejecuta sin la aprobación prometida

Contexto: `omega full --unattended` pone `f.yes = true`
(`internal/cli/full.go:428`). Eso activa `ModeAuto`, y entonces
`authorize` (`internal/agent/agent.go:592-603`) aprueba sin preguntar todo lo
Dangerous que no sea `AlwaysConfirm`. Por eso, con `--yes` o en desatendido,
todo lo que se escapa de `AlwaysConfirm` se ejecuta sin nadie delante.

- [x] **A1. Los borrados en `$HOME` con ruta relativa o tras un `cd` no
  piden confirmación siempre.**
  - Dónde: `alwaysAsks` (`internal/safety/ask.go:30-34`, `:162`) mira cada
    segmento por separado y no sigue los `cd`. `cdTarget` solo lo usa
    `wipesAnywhere`.
  - Sondeo: todos estos dan Dangerous con `always=false`:
    - `cd ~ && rm -rf Documents`, `cd ~/Documents && rm -rf *`,
      `(cd ~/Documents && rm -rf x)`;
    - `rm -rf node_modules`, `rm -rf ./*`, `echo ~/Documents | xargs rm -rf`;
    - `cd ~/Documents && find . -delete`, `git clean -fdx`.
  - En cambio, la ruta absoluta equivalente sí pregunta siempre. Es el mismo
    caso que el incidente de `node_modules` de la cuarta revisión, que el
    arreglo de entonces no cubre.
  - Debe:
    - llevar el directorio de trabajo de un segmento al siguiente (`cd`,
      `pushd`, subshell) reutilizando `cdTarget`/`rebase`;
    - si el directorio es desconocido o está en `$HOME`, tratar como
      «confirmar siempre» `rm`, `find -delete`, `git clean` y `xargs rm` con
      ruta relativa.
  **Hecho:** `alwaysAsks` sigue `cd`/`pushd`/subshells entre segmentos; con directorio desconocido o en `$HOME`, `rm`, `find -delete`, `xargs rm` y `git clean` con ruta relativa preguntan siempre (`internal/safety/ask_home.go`).
- [x] **A2. Tampoco preguntan siempre los borrados no recursivos ni las
  sobrescrituras en `$HOME`.**
  - Sondeo, todos con `always=false`: `rm -f ~/Documents/*.pdf`,
    `rm -f ~/.zshrc`, `find ~/Documents -name '*.pdf' -delete`,
    `rsync -a --delete /tmp/empty/ ~/Documents/`,
    `: > ~/Documents/thesis.tex`, `truncate -s0 …`, `shred -u …`.
  - Ni las operaciones de git que pierden trabajo: `git reset --hard`,
    `git clean -fdx`, `git stash clear`, `git push --force`,
    `git checkout -- .`.
  - Debe:
    - en `alwaysAsks`, tratar todo eso como un borrado recursivo cuando
      apunta a `$HOME` fuera de las cachés de `regenerable`;
    - en `gitAsks`, añadir `reset --hard`, `clean -f*`, `stash drop|clear`,
      `push --force*` y `checkout -- .`.
  **Hecho:** también `rm` no recursivo, `rsync --delete`, `shred`, `truncate`, `tee` y `>` hacia `$HOME`; `gitAsks` añade `reset --hard`, `clean -f`, `stash drop|clear`, `push --force`, `checkout -- .`, `restore` y `branch -D`.
- [x] **A3. Reiniciar, apagar o cerrar la sesión solo está prohibido en el
  prompt** (`internal/agent/prompt.go:322`, `:347`); la política no lo
  aplica.
  - Sondeo: todos estos dan Dangerous con `always=false`:
    - `shutdown -r now`, `reboot`, `systemctl poweroff`, `launchctl reboot`;
    - `osascript … shut down`;
    - `softwareupdate --install --all --restart`;
    - `kill -9 -1`, `killall -9 loginwindow`;
    - `Restart-Computer -Force`, `shutdown /r /t 0`.
  - Debe: bloquearlos, incluido `softwareupdate … --restart|-R`. Si no, al
    menos «confirmar siempre».
  **Hecho:** bloqueados (Blocked) en `internal/safety/power.go`; `softwareupdate --list` sigue siendo Safe.
- [x] **A4. Se puede sacar una clave privada empaquetándola.**
  - `tar -cf - ~/.ssh | base64` y `zip -r - ~/.ssh` dan Dangerous, mientras
    que `cat`, `xxd` o `cp` de `~/.ssh/id_rsa` dan Blocked.
  - Con `--yes`, el modelo y el registro reciben la clave en base64, y
    `pemMask` no la reconoce.
  - Dónde: `leaksIn`/`recursiveReader` (`internal/safety/secret.go:685-760`)
    no conocen los archivadores.
  - Debe: tratar `tar`, `zip`, `7z`, `cpio`, `pax`, `ditto -c` y `rsync`
    sobre una ruta secreta como lectores recursivos (Blocked).
  **Hecho:** `tar`, `zip`, `7z`, `pax`, `cpio`, `ditto -c` y `rsync` sobre rutas secretas dan Blocked (`internal/safety/archive.go`). Efecto conocido: empaquetar todo `~` también queda bloqueado.
- [x] **A5. Borrar VMs, clústeres, modelos o el crontab no se trata como
  `docker system prune --volumes`.**
  - `always=false` en `colima delete`, `limactl delete`, `orb delete`,
    `multipass purge`, `vagrant destroy -f`, `minikube delete --all`,
    `kind delete clusters --all`, `ollama rm`, `conda env remove`,
    `crontab -r` y `security delete-generic-password`.
  - Y en los destructivos remotos: `gh repo delete --yes`,
    `aws s3 rm --recursive`, `kubectl delete`,
    `terraform destroy -auto-approve`.
  - El criterio es incoherente: `podman machine rm` sí pregunta siempre.
  - Debe: ampliar `containerDeletes`/`alwaysAsks` con estos verbos.
  **Hecho:** `internal/safety/ask_destroy.go` cubre VMs, clústeres, modelos, `crontab -r`, `security delete-*` y los destructivos de gh, aws, kubectl, helm, terraform/tofu, pulumi y gcloud/az.
- [x] **A6. En Windows, la tarea programada vacía la Papelera de
  reciclaje.**
  - `config.Skipped` (`internal/config/config.go:237`) solo excluye el id
    `trash`, pero en Windows el paso se llama `recycle-bin`
    (`internal/steps/steps.go:546`).
  - `windowsTaskArgs` (`internal/scheduler/scheduler.go:231`) no pone
    `OMEGA_SCHEDULED=1`, porque `schtasks /TR` no lleva variables de
    entorno.
  - Resultado: el `clean --yes --no-ai` semanal ejecuta
    `Clear-RecycleBin -Force`, aunque `Describe` promete lo contrario.
  - Debe:
    - un flag `--scheduled` en los argumentos de la tarea en vez de la
      variable de entorno;
    - que `Skipped` cubra `trash` y `recycle-bin`;
    - una prueba por sistema.
  **Hecho:** flag oculta `--scheduled` en launchd, systemd y schtasks (`scheduler.FlagScheduled`), `config.Scheduled()`, y `Skipped` excluye `trash` y `recycle-bin`. Las tareas de Windows creadas por versiones anteriores se actualizan solas: al arrancar, `scheduler.UpgradeTask` lee la tarea con `schtasks /Query /XML` y, si tiene los argumentos antiguos, le añade `--scheduled` con `schtasks /Change` (una sola vez, con marca en el directorio de registros); mientras tanto `IsLegacyRun` trata esa ejecución como programada.
- [x] **A7. `dev_activate` escribe en `~/.zshrc` sin confirmar siempre.**
  - Llama a `authorize(..., Dangerous, false)` (`internal/agent/omega.go:171`).
  - `write_file` sobre el mismo fichero sí pregunta siempre
    (`sensitiveWrite`, `tools.go:886`).
  - Debe: `always=true`, y denegarlo en desatendido.
  **Hecho:** `dev_activate` pregunta siempre y se deniega en desatendido.
- [x] **A8. `omega_settings set` y `notes forget` cambian estado duradero
  sin nadie delante** (`internal/agent/omega.go:460`, `:387`, con
  `Dangerous, false`).
  - En desatendido el modelo puede cambiar `model`, apagar `security` con
    `skip_steps`, tocar `command_timeout` o borrar notas.
  - Contradice a `sensitiveCommand` (`tools.go:659-665`): la configuración
    de omega «se pregunta siempre».
  - Debe: `always=true`, y denegarlo en desatendido.
  **Hecho:** `omega_settings set` y `notes forget` preguntan siempre y se deniegan en desatendido.
- [x] **A9. Los pasos del catálogo esquivan el `AlwaysConfirm` de su propio
  comando.**
  - `toolRunStep` pasa solo `s.AlwaysConfirm` (`tools.go:554`), y
    `needsAnswer` exime a los pasos integrados
    (`internal/cli/deterministic.go:255`).
  - En estos pasos el catálogo dice `false` y `safety.AlwaysConfirm(cmd)`
    dice `true`:
    - `mole-purge` (`mo purge --yes`);
    - `omz`;
    - `macos` (sudo);
    - los pasos apt, dnf, pacman, snap y journal con sudo.
  - Consecuencia: el `clean` programado en el Mac ejecuta `mole-purge` sin
    nadie delante, es decir, borra `node_modules`, `.venv` y `target`.
  - Afecta también a `omega custom` y a `omega steps run`, que comparten
    `needsAnswer`.
  - Debe: `always = s.AlwaysConfirm || safety.AlwaysConfirm(s.Command)` en
    el agente y en `needsAnswer`, y marcar `mole-purge` como
    `AlwaysConfirm`.
  **Hecho:** el agente y `needsAnswer` usan `s.AlwaysConfirm || safety.AlwaysConfirm(cmd)`; `mole-purge` es AlwaysConfirm. En la CLI, los pasos integrados que solo preguntan por `sudo` siguen exentos cuando hay sesión de sudo, para que la actualización programada funcione; en el agente, `omz` y `macos` preguntan siempre.
- [x] **A10. En desatendido, `propose_actions` con varias opciones sigue
  tomando casi todo.**
  - `autoChoose` (`tools.go:1122-1129`) toma toda opción cuyo `risk` no sea
    `dangerous`, y ese campo es opcional y lo pone el modelo.
  - Unido a `mac_change` (`internal/agent/mac.go:128-135`, `app-remove:` y
    `disable:` con `always=false`), una desinstalación de app que no se
    puede deshacer se ejecuta sola.
  - Debe:
    - tomar solo las opciones `recommended=true` cuyo `risk` explícito sea
      `read-only` o `normal`;
    - denegar en desatendido `app-remove`, `disable` y `perm-reset-all`.
  **Hecho:** `autoChoose` solo toma opciones `recommended` con riesgo explícito `read-only`/`normal`; `mac_change` deniega `app-remove`, `disable` y `perm-reset-all` en desatendido.

### B. Órdenes de la CLI que hacen daño o engañan

- [x] **B1. `omega net kill <puerto>` mata con `kill -9` sin confirmar ni
  decir qué proceso es** (`internal/netdiag/netdiag.go:293-317`,
  `internal/cli/net.go:89-104`).
  - Debe:
    - mostrar «PID 673 ControlCenter en :5000»;
    - pedir confirmación, o `--yes`;
    - enviar SIGTERM, esperar unos segundos y solo después SIGKILL, como
      hace `agents kill`.
  **Hecho:** muestra PID, proceso y puerto, pide confirmación o `--yes`, SIGTERM, espera 3 s y luego SIGKILL; en Windows `taskkill` y después `/F`.
- [x] **B2. `omega git status` y `git clean` dan por fusionadas las ramas
  según la rama actual, no la de por defecto**
  (`internal/gitops/gitops.go:159`, `:166`).
  - Solo protegen `main`, `master` y `develop`.
  - En `~/Dev/doctorSIM/www`, con HEAD en una rama de feature, propone
    borrar `develop2`, `release/fiscal-panel-prod-10253`,
    `backup-develop2-…` y 317 ramas en total.
  - Debe:
    - comparar con `origin/HEAD` o con la rama por defecto;
    - proteger `release/*`, `develop*`, `backup*` y la rama upstream de
      cada worktree;
    - avisar cuando HEAD no es la rama por defecto;
    - tener `--dry-run`.
  **Hecho:** comparación con la rama por defecto, ramas protegidas (`develop*`, `release*`, `backup*`, worktrees y sus upstream), aviso si HEAD no es la de por defecto, `--dry-run`/`--yes` y una sola pregunta con `ui.Pick`.

---

## P1

### C. Resultados equivocados o engañosos

- [x] **C1. `disk duplicates` sobre una carpeta que no existe dice «No
  duplicate files found» y sale con 0.**
  - Causa: `filepath.Walk` ignora todos los errores
    (`internal/cleaner/cleaner.go:301-304`).
  - Debe: validar la carpeta, como `disk large`, y decir qué carpetas no se
    pudieron leer (TCC en `~/Downloads`).
  **Hecho:** valida la carpeta, sale con 1 si no existe y cuenta las carpetas ilegibles.
- [x] **C2. `net ping` muestra «404 OK».** Cualquier respuesta HTTP cuenta
  como OK (`internal/cli/net.go:43-46`). En macOS además lista «Ubuntu
  Mirrors».
  - Debe: decir «alcanzable (HTTP 404)» o usar URLs que den 200, y filtrar
    los destinos por sistema.
  **Hecho:** «alcanzable (HTTP 404)»; destinos filtrados por sistema; comprobaciones en paralelo.
- [x] **C3. `agents list` invita a matar cuando no hay nada que matar.**
  - Dice «Left behind: 0 processes … holding 0 B» y, debajo, «… 57 MB» por
    grupo, porque la cabecera excluye los daemons compartidos y el grupo los
    incluye.
  - Cierra con «`omega agents kill` ends them all» (`agents.go:331-347`).
  - Debe:
    - no dar la pista si no hay nada;
    - usar el mismo criterio de memoria en cabecera y grupos;
    - agrupar en una línea los 30 `git fsmonitor--daemon` iguales.
  **Hecho:** sin pista si no hay nada; mismo criterio de memoria; daemons compartidos agrupados («29 × git fsmonitor--daemon»).
- [x] **C4. El paso `brew` (y `brew-greedy`) falla con `pipefail` cuando no
  queda nada que actualizar** (`internal/steps/shell.go:80`, `:86`).
  - `… | grep -vxF "$gone" | xargs -r …`: si grep no deja ninguna línea,
    sale con 1. Pasa siempre que hay un cask «gone» o de sudo y nada más
    pendiente.
  - Además dispara la escalada a Sonnet.
  - Debe: `{ grep -vxF "$gone" || true; }`, con una prueba de la tubería.
  **Hecho:** `{ grep -vxF "$gone" || true; }`, con prueba bajo `pipefail` en bash y zsh.
- [x] **C5. `disk_usage` sale con 1 en cuanto hay una carpeta ilegible**
  (`tools.go:708`, `du … | sort | head` con `pipefail`).
  - En macOS ocurre siempre. Al no cachearse, el escaneo de 1-2 min se
    repite, y el modelo lo toma por un fallo.
  - Debe: ignorar el código de salida de `du`.
  **Hecho:** `{ du … || true; } | sort | head`.
- [x] **C6. La caché de la sesión devuelve resultados viejos después de un
  cambio hecho fuera de `runShell`.**
  - `forget()` solo se llama allí (`tools.go:234-237`). Tras reescribir
    `topgrade.toml` con `write_file`, `run_step topgrade-preview` contesta
    «nothing has changed since» (`tools.go:313`).
  - Al conceder un permiso de macOS, `regainAccess` (`access.go:124`) solo
    limpia `macInv`.
  - Debe: llamar a `a.forget()` en todo handler que escribe y antes de
    `rerun()`.
  **Hecho:** `forget()` en todo lo que escribe y antes de `rerun()` en `regainAccess`.
- [x] **C7. Los pasos de `steps.d` imponen el riesgo que declaran.**
  - `risk = "read-only"` con un `rm -rf` dentro se ejecuta en `--dry-run`,
    en solo lectura y en `analyze`/`health`
    (`internal/customsteps/customsteps.go:90-116`,
    `deterministic.go:338`).
  - Debe: para los pasos no integrados, `risk = max(declarado,
    Classify(cmd))`.
  **Hecho:** el riesgo es el mayor entre el declarado y `safety.Classify(cmd)`.
- [x] **C8. `audit secrets` promete «dotfiles» y solo lee historiales de
  bash y zsh** (`secscan.go:228-234`).
  - No lee el historial de fish ni el de atuin, que esta máquina usa.
  - Sale con 0 aunque encuentre fugas.
  - Debe: decir lo que revisa de verdad o ampliar las fuentes, y salir con
    un código distinto de 0 si encuentra algo.
  **Hecho:** lee también fish, atuin y los historiales de python, node, psql y mysql; dice qué leyó; sale con 1 si encuentra algo (`--no-fail`).
- [x] **C9. `rollback` dice «Inspect and restore» y solo lista.**
  - Si no hay instantáneas sale vacío, sin decirlo.
  - En Linux lanza `sudo snapper list` con 5 s de límite, y puede quedarse
    esperando la contraseña (`rollback.go:15`, `customsteps.go:225-240`).
  **Hecho:** `rollback` lista con `tmutil`, `snapper` (con `sudo -n`, sin esperar contraseña) o `Get-ComputerRestorePoint`, dice «No hay instantáneas» y tiene `--json`.
- [x] **C10. `--max-cost` deja de funcionar en silencio cuando el precio del
  modelo es desconocido** (`agent.go:338-357`): modelos custom, una
  escalada a un modelo sin precio, el catálogo apagado.
  - Debe: avisar al empezar y cortar por tokens o por `--max-turns`.
  **Hecho:** con precio desconocido, `--max-cost` cuenta a `llm.CautiousPrice` ($10/$50 por millón), avisa una vez y corta (`internal/agent/costlimit_test.go`).

### D. Coste y robustez

- [x] **D1. Con Haiku 5.5 se paga ×5 entre 100k y 140k tokens antes de
  compactar.**
  - `CompactAt` es 140k para `claude-*` (`internal/llm/llm.go:151-153`),
    pero `longPrompt` multiplica por 5 a partir de 100k
    (`pricing.go:29-31`).
  - Debe: `CompactAt = min(70 % de la ventana, LongAbove − margen)`.
  **Hecho:** `CompactAt` = mínimo entre el 70 % de la ventana y el umbral de precio largo menos 10k (90 000 con Haiku 5.5).
- [x] **D2. El cliente de Anthropic no tiene límite de tiempo**
  (`internal/llm/anthropic.go:26`); el de OpenAI tiene 10 min.
  - Una conexión colgada bloquea para siempre una ejecución programada.
  - Debe: `option.WithRequestTimeout(10*time.Minute)` por intento.
  **Hecho:** `WithRequestTimeout(10m)` por intento; un tiempo agotado no se reintenta.
- [x] **D3. La escalada de modelo es para siempre y salta también con un
  Ctrl-C o un tiempo agotado.**
  - `escalate` deja `escalated = true` (`agent.go:550-563`), y `runShell`
    escala con exit −1 (`tools.go:261`).
  - Debe: no escalar por cancelación ni por tiempo agotado, y volver al
    modelo rápido tras N turnos sin fallos.
  **Hecho:** no escala por cancelación ni tiempo agotado; vuelve al modelo inicial tras 4 turnos sin fallos.
- [x] **D4. Los proveedores compatibles con OpenAI no cuentan la caché.**
  - `oaResponse` ignora `prompt_tokens_details.cached_tokens`
    (`openai.go:150-153`, `:230`).
  - Con OpenRouter→Claude no se envía `cache_control`, así que cada turno
    reenvía el historial sin caché y `--max-cost` corta antes de tiempo.
  **Hecho:** `cached_tokens` como `CacheRead`; `cache_control` en el sistema y el último mensaje de usuario solo para OpenRouter con `anthropic/*`, con el formato y los campos que documenta OpenRouter (bloques con `cache_control` en mensajes de sistema y de usuario; `prompt_tokens_details.cached_tokens` y `cache_write_tokens`).
- [x] **D5. `list_steps` devuelve unos 24 KB (~6k tokens) en cada llamada**
  (`tools.go:501-525`): todos los pasos, instalados o no, con su comando
  completo.
  - Debe: por defecto solo los disponibles y sin comando; `detail=true`
    para uno concreto.
  **Hecho:** por defecto solo pasos disponibles y sin comando; `id`, `all` y `detail` para más.
- [x] **D6. Las notas se inyectan enteras en cada system prompt**
  (`agent.go:142`, `prompt.go:443-450`). Aquí ya son 93 notas, muchas
  repetidas («Worktree agents-v3.7 pendiente» aparece en 15).
  - Debe:
    - meter solo las N más recientes, o hasta ~4 KB;
    - no guardar una nota igual a otra (normalizada y sin fecha);
    - una orden `omega notes compact` que proponga fusionar las repetidas.
  **Hecho:** el prompt lleva como mucho ~4 KB de notas, las más nuevas primero; `remember` no repite notas; `omega notes compact` (N4).
- [x] **D7. Los pendientes nunca caducan** (`pending.go:174-197`).
  - Los `done` y `dismissed` se acumulan, y un `open` de hace meses sigue en
    el prompt.
  - Debe:
    - borrar los `done` de más de 30 días;
    - pasar a `stale` los `open` que nadie ha visto en 30 días;
    - limitar la lista del prompt.
  **Hecho:** `done` de más de 30 días se borran, `open` sin ver en 30 días pasan a `stale`, y el prompt muestra 15 con «+N more».
- [x] **D8. Ctrl-C y los tiempos agotados matan con SIGKILL al instante**
  (`internal/runner/session_unix.go:18`). Deja `apt`, `dpkg`, `brew` o
  `softwareupdate` a medias, con bloqueos.
  - Debe: SIGTERM al grupo, una espera de unos segundos y luego SIGKILL.
  - Afecta también a `omega custom`.
  **Hecho:** SIGTERM al grupo, 5 s y luego SIGKILL (Windows igual que antes).
- [x] **D9. `--dry-run` y el modo solo lectura siguen escribiendo notas y
  pendientes** (`tools.go:971-982`, `pendingtool.go:56-83`).
  **Hecho:** en solo lectura o `--dry-run` no se guardan notas ni pendientes, y la herramienta lo dice.
- [x] **D10. Hay fuentes de texto no confiable que no marcan la sesión como
  «tainted».**
  - `run_history` devuelve 12 KB de transcripciones antiguas que incluyen
    texto de páginas web (`omega.go:267`, `log.go:551`).
  - Un `curl` aprobado tampoco la marca.
  **Hecho:** `run_history` y los `curl`/`wget`/`gh api`/`iwr` aprobados marcan la sesión.
- [x] **D11. `effort = "High"` se valida en minúsculas pero se envía tal
  cual** (`config.go:135` frente a `root.go:403`, `model.go:288`). La API
  probablemente responde 400.
  - Debe: normalizar el valor al cargar la configuración.
  **Hecho:** `effort` se pasa a minúsculas al cargar y al validar.
- [x] **D12. Las claves desconocidas de `config.toml` se ignoran sin
  avisar** (`config.go:97` descarta `Undecoded()`). Una errata como
  `aproval = "all"` deja la política por defecto sin que nadie lo sepa.
  - Debe: avisar en `config check` y en `doctor`.
  **Hecho:** `config.UnknownKeys()`; avisan `config check` y `doctor`; `disk_budgets` se valida.
- [x] **D13. `write_file` no es atómico** (`tools.go:911`): con el disco
  lleno deja el fichero truncado.
  - Debe: escribir en un temporal y renombrar.
  **Hecho:** temporal en el mismo directorio, fsync y rename; conserva permisos, sigue el symlink y mantiene `.omega-bak`.

---

## P2: coherencia de la CLI

### E. Idioma

- [x] **E1. El modo sin IA está entero en inglés** (`deterministic.go:277-545`).
  - Ejemplos: «⚡ Omega Deterministic Mode: Update», «Running the safe
    cleanups…», «skipped: needs a confirmation at a terminal», «No problems
    found.», «💡 Run 'omega update'…».
  - También el `Summary` del JSON y el aviso «omega: running without AI»
    (`root.go:602`).
  - Es lo que ve cualquier ejecución programada.
  **Hecho:** modo sin IA y `Summary` del JSON traducidos.
- [x] **E2. Unas 15 órdenes tienen ayuda y salida solo en inglés**: `audit`,
  `privacy`, `schedule`, `uninstall`, `disk`, `git`, `net`, `rollback`,
  `shell`, `quick`, `man`, `skills`, `assist`, `agents`, `export` y
  `bootstrap`. Ninguna llama a `i18n.T`.
  - En `omega --help` conviven «quick  One-click safe maintenance» y «fix
    Diagnosticar…».
  **Hecho:** todas las órdenes de herramientas pasan por i18n (`messages_tools.go`). Los textos de las comprobaciones de `assist` (Title/Why) siguen en inglés.
- [x] **E3. Las plantillas de cobra siguen en inglés**: «Usage:», «Flags:»,
  «Available Commands:», «help for X», «Help about any command». Y también
  sus errores: «unknown flag», «accepts at most 1 arg(s)».
  - Debe:
    - traducir las plantillas con `SetUsageTemplate`/`SetHelpTemplate`;
    - traducir los errores con `SetFlagErrorFunc` y validadores propios;
    - terminar cada error con «omega X --help».
  **Hecho:** plantillas de ayuda, errores de opciones y de argumentos traducidos, con sugerencia y «omega X --help» (`internal/cli/cobrahelp.go`).
- [x] **E4. 47 de los 122 pasos no tienen nombre traducido**: «Stale project
  build folders», «Shell startup time», «Go module cache», «Old gem
  versions»…
  - Faltan claves `step.<id>`, por ejemplo para `dev-artifacts`,
    `shell-startup`, `go-modcache`, `gem-cleanup`, `ios-device-support`,
    `xcode-derived`, `yarn-cache` y `mise-prune`.
  **Hecho:** 34 pasos con nombre traducido y una prueba que lo exige.
- [x] **E5. No hay plurales.** Salen «visto en 1 ejecuciones», «30
  registro(s)», «1 unpushed commit(s)» y «comprobación(es) fallida(s)».
  - Debe: `i18n.N(key, n)` con formas `one` y `other`.
  - Esta pasada evitó el problema en los textos nuevos («ejecuciones: 1»,
    «fallos: 2»), pero los antiguos siguen igual.
  **Hecho:** `i18n.N` con formas `.one`; sin «(s)» en los textos.
- [x] **E6. Hay textos fijos en inglés dentro del agente**: «Reached cost
  budget limit» (`agent.go:356`) y los errores de `ParseFor`
  (`pending.go:220`) y de `pending.Resolve`.
  **Hecho:** `agent.cost_limit`, `ParseFor`, `Resolve` y `Set` traducidos.
- [x] **E7. El sistema de i18n se apoya en mapas sueltos**
  (`messages*.go`, 5 idiomas por clave) sin comprobar los `%` de formato.
  - Una traducción con un `%d` de menos o de más imprime `%!d(MISSING)`.
  - Debe: una prueba que compare los verbos de formato de los 5 idiomas.
  **Hecho:** prueba que compara los verbos de formato (también con índices `%[2]s`) entre idiomas y entre `.one` y su base.

### F. Órdenes, opciones y ayudas

- [x] **F1. `omega fix` hereda opciones que no puede usar** (`fix.go:47`):
  `--no-ai`, `--json`, `--format`, `--fail-on-*`, `-q` y `-v` hacen fallar
  la orden.
  - Además describe `fix` de tres formas distintas (`messages.go:44`,
    `cmd.fix`, `fix.go:20` en inglés).
  **Hecho:** `fix` solo registra opciones del agente; descripción única que documenta `omega fix -` y `--log`.
- [x] **F2. Las opciones globales «with --no-ai: …» salen en todas las
  ayudas sin agrupar.**
  - `-v` es «verbose» solo sin IA.
  - `--max-cost` y `--max-turns` no dicen que 0 significa sin límite.
  - Debe: dos bloques, «Opciones del agente» y «Opciones sin IA», y un aviso
    si se usan con IA.
  **Hecho:** bloques «Opciones del agente» y «Opciones sin IA»; 0 = sin límite; aviso si se usa una opción sin IA con IA.
- [x] **F3. Los códigos de salida no están documentados.**
  - `--fail-on-outdated` y `--fail-on-warning` salen con 2.
  - Los errores de uso salen con 1, igual que un fallo
    (`deterministic.go:222-239`).
  - `--fail-on-critical` significa «falla cualquier paso».
  - Debe: 0 correcto, 1 fallo, 2 avisos y 64 uso, documentados en `--help`
    y en el man, y aclarar `--fail-on-critical`.
  **Hecho:** 0, 1, 2 y 64, documentados en la ayuda, el man y el manual.
- [x] **F4. Errores de escritura sin buena respuesta.**
  - Las subórdenes mal escritas no tienen sugerencia: `omega config sett` →
    `unknown command "sett"` (`group()`, `root.go:196-200`).
  - `omega dsk` sugiere `disk`, pero sigue con «este omega no tiene… actualízalo
    con brew upgrade».
  - `omega help foo` imprime la ayuda raíz y sale con 0.
  - Debe:
    - usar `suggest()` en todos los niveles;
    - el texto de «actualiza» solo cuando no hay nada parecido;
    - `help foo` debe fallar.
  **Hecho:** sugerencias en todos los niveles; «actualiza omega» solo sin sugerencia; `help foo` sale con 64.
- [x] **F5. `update`, `clean`, `analyze`, `health` y `tune` son palabras de
  la raíz, no órdenes**: `omega health --help` muestra la ayuda raíz.
  - Debe: convertirlas en órdenes con su propia ayuda, como `full`, `quick`
    y `fix`.
  **Hecho:** `update`, `clean`, `analyze`, `health` y `tune` son órdenes con su ayuda.
- [x] **F6. Hay conceptos duplicados o mal nombrados.**
  - Para el modelo hay cuatro sitios: `model`, `models`, `config set model`
    y `setup`.
  - El historial se reparte en tres: `logs`, `pending` y `notes`.
  - El atajo `install` lleva a `dev install`, mientras que `uninstall`
    desinstala apps.
  - El atajo `backup` lleva a `export`, que no es una copia de seguridad.
  - Debe:
    - `models` como `model list`, o al menos «Ver también» en cada ayuda;
    - un `omega history` que reúna logs, pending y notes;
    - quitar el atajo `backup`.
  **Hecho:** `omega model list` (con `models` oculto), «Ver también», sin atajo `backup`, y `omega history` (N3).
- [x] **F7. Solo `pending` y `custom` tienen ejemplos (`Example:`)**.
  - Les faltan a `net`, `git`, `disk`, `audit`, `schedule`, `rollback`,
    `uninstall`, `logs`, `config`, `steps run`, `model`, `models` y
    `export`.
  **Hecho:** ejemplos en todas las órdenes de la lista y alguna más.
- [x] **F8. Los `Short` demasiado largos rompen la columna de `omega
  --help`**: `full` tiene unos 150 caracteres, y también `mac`, `assist` y
  `auth`.
  - Debe: 60 caracteres como máximo; el resto, en el Long.
  **Hecho:** todos los Short caben en 60 columnas en los 5 idiomas, con prueba.
- [x] **F9. La completion de la shell es pobre.**
  - Está oculta (`completion.go:20`).
  - Usa bash v1 y no tiene descripciones.
  - No completa valores: `--provider`, `--effort`, `--format`,
    `schedule install --task`, los ids de `pending`, `logs view` o
    `steps run`. Esta pasada sí añadió la completion de ids a
    `custom --set`.
  - Debe: `GenBashCompletionV2`, más `RegisterFlagCompletionFunc` y
    `ValidArgsFunction` donde haga falta.
  **Hecho:** completion visible, bash V2 con descripciones y valores para opciones e ids.
- [x] **F10. `omega man` escribe `./man/omega.1` en el directorio actual**,
  aunque el pie de cada ayuda lo presenta como «Todos los comandos y
  opciones».
  - Debe: mostrarlo (`man -l` o stdout), y escribir el fichero solo con
    `--output`.
  **Hecho:** `omega man` muestra la página; `--output DIR` la escribe; siempre en inglés; incluye EXIT STATUS.
- [x] **F11. Falta `--json` en casi todo.**
  - Lo tienen `steps`, `doctor`, `agents`, `skills`, `assist` y
    `disk limits`.
  - Les falta a `logs`, `pending`, `notes`, `net`, `audit`, `privacy`,
    `git status`, `disk large|duplicates|artifacts`, `docker`,
    `schedule status`, `rollback`, `models`, `auth status`, `config show` y
    `custom --show`.
  **Hecho:** `--json` en todas las órdenes de la lista.
- [x] **F12. Órdenes lentas sin señal de progreso.**
  - `disk large` (por defecto `~`) pasa más de 90 s sin decir nada.
  - `git status` tarda 49 s con un solo «Scanning dev directories…».
  - Debe:
    - un contador o spinner en stderr, y `[3/11] www…` por repositorio;
    - Ctrl-C con resultados parciales;
    - saltar `~/Library` y las carpetas de nube por defecto.
  **Hecho:** progreso en stderr (solo en terminal), Ctrl-C con resultados parciales, y `disk large` salta `~/Library` y la nube salvo con `--all`.
- [x] **F13. Listas sin límite.** `git status` imprime en una sola línea de
  unos 15 000 caracteres las 317 ramas fusionadas.
  - Debe: mostrar 10, «y 307 más», y ofrecer `--all`.
  **Hecho:** 10 ramas por repositorio, «y N más» y `--all`.
- [x] **F14. `disk artifacts` pregunta «¿Borrar todos?» sin dejar elegir**
  (`disk.go:139-152`).
  - Debe: `ui.Pick`, como `docker`, más `--dry-run` y `--yes`.
  **Hecho:** `ui.Pick`, confirmación final con el tamaño, `--dry-run`, `--yes` y `--json`.
- [x] **F15. `disk duplicates` y `disk large` esconden su carpeta por
  defecto** (`~/Downloads` y `~`).
  - El mínimo de 1 MB es fijo.
  - `large` recorta a 25 resultados sin avisar (`disk.go:89`).
  **Hecho:** carpeta por defecto documentada, `--min`, `--limit` y aviso de recorte.
- [x] **F16. `net ports` corta los nombres de proceso** («ControlCe»,
  «DropboxFi») porque `lsof` va sin `+c 0` (`netdiag.go:207`).
  - La columna «Protocol» siempre dice TCP y no ordena por puerto.
  **Hecho:** `lsof +c 0`, orden por puerto, protocolo real y `--udp`.
- [x] **F17. `export` sobrescribe sin preguntar** (`blueprint.go:152`) y no
  admite `-` para escribir a stdout.
  **Hecho:** pregunta antes de sobrescribir, `--force`, y `-` escribe a stdout.
- [x] **F18. `schedule status` no dice qué se ejecuta, cuándo ni cómo fue
  la última vez.**
  - Debe: mostrar tarea, intervalo, próxima ejecución, último resultado y
    registro.
  - Además, las ayudas de `install` empiezan en mayúscula y están en inglés.
  **Hecho:** tarea, intervalo, próxima ejecución, último resultado y registro.
- [x] **F19. `docker` en modo no interactivo y con Docker parado sale con 0
  sin decir nada útil**, y no tiene un modo de solo listar (`--list` o
  `--json`).
  **Hecho:** mensaje claro y salida 1 con Docker parado sin terminal; `--list` y `--json`.
- [x] **F20. Unidades de tamaño inconsistentes.**
  - Hay tres `humanBytes`: `ui/panel.go`, `cli/docker.go` (decimal, 1e9) y
    `assist.Human`.
  - Más `ui.Bytes`, añadido en esta pasada.
  - Debe: un único formateador, y coma decimal en español.
  **Hecho:** un único `ui.Bytes` (con coma decimal en es/fr/de/pt); quitados los demás formateadores.
- [x] **F21. `steps` no tiene filtros ni detalle**: faltan `--installed`,
  `--category` y `omega steps show <id>`, que mostraría el comando completo,
  qué hace, el riesgo y cómo instalarlo.
  **Hecho:** `steps --installed`, `--category` y `steps show <id>` (N2).
- [x] **F22. Varias órdenes escriben con `fmt.Print` en vez de
  `cmd.OutOrStdout()`**: `man`, `uninstall`, `config show|path` y
  `steps run`. Así no se pueden probar ni reutilizar desde la app.
  **Hecho:** todas escriben en `cmd.OutOrStdout()`.

---

## P2: interfaz de terminal

### G. Coherencia visual

- [x] **G1. Los glifos `⏺`, `⎿` y `⏵⏵` se ven mal o como cuadros vacíos**
  en muchos terminales.
  - `⏺` sale como emoji naranja pegado al texto en `running.png`,
    `confirm.png` y `summary.png`.
  - Dónde: `tui.go:440-478`, `:1076`, `plain.go:72-92`, `pilot.go:745`.
  - Debe:
    - un `glyphs.go` central con `●`, `└` y `»`;
    - una variante ASCII con TERM=dumb u `OMEGA_ASCII=1`;
    - regenerar las capturas y `pilot_test.go:65`.
  **Hecho:** `internal/ui/glyphs.go` (●, └, »…) con variante ASCII (`TERM=dumb`, `OMEGA_ASCII=1`). Capturas de terminal (`scripts/capture.sh`) y de la app (`scripts/capture_gui.mjs`) regeneradas con los glifos nuevos; `sudo.png` necesita root y `runuser`, así que la regenera `captures.yml`.
- [x] **G2. La marca de la app no es una sola**: «✻ omega» en TUI, menú,
  guía y piloto, y «Ω» en los informes de texto y en la mascota.
  - Debe: una constante `brand` y elegir una.
  **Hecho:** una sola marca «Ω omega».
- [x] **G3. La paleta se usa fuera de su sitio.** Hay `NewStyle().Foreground(…)`
  creados sobre la marcha en `tui.go:353` (con hex fijos), `tui.go:457`,
  `panel.go:80`, `picker.go:278`, `guide.go:639` y `pilot.go:692-878`.
  - `report.go` repite la paleta con su propio renderer.
  - Debe: un `theme.go` con todos los estilos.
  **Hecho:** `internal/ui/theme.go` con la paleta y todos los estilos; `report.go` usa la misma paleta.
- [x] **G4. Las pistas de teclas usan formatos distintos**: «[y] yes
  [a]…» (corchetes y tres espacios), «enter confirm · esc…», «↑↓» frente a
  «↑/↓», «ctrl+c cancels» frente a «stops».
  - Debe: un solo formato, «tecla acción · tecla acción», con la tecla en
    negrita.
  **Hecho:** `hintLine`/`cardHints` con un solo formato en todas las pantallas (`internal/ui/hints.go`).
- [x] **G5. Las pistas tienen poco contraste.** `faint` da unos 2,5:1 sobre
  blanco y 3:1 sobre negro.
  - Debe: pistas en `muted` con la tecla en negrita, y `faint` solo para lo
    decorativo.
  **Hecho:** pistas en `muted` con la tecla en negrita.
- [x] **G6. Un mismo atajo significa cosas distintas según la pantalla.**
  - ctrl+p sube en el menú, cambia de modelo en la sesión y va a la
    coincidencia anterior en la búsqueda.
  - La lista del menú da la vuelta al llegar al final y la del picker no.
  - `q` sale del menú, pero en la sesión inactiva hace falta Esc dos veces.
  - Debe: un `keymap.go` común (`bubbles/key`).
  **Hecho:** `keymap.go`; ctrl+p solo cambia de modelo; todas las listas dan la vuelta. Dos cosas se dejan a propósito: `q` no sale de la sesión porque es una letra que se escribe (Esc sale con la entrada vacía), y el menú no usa j/k porque escribir filtra.
- [x] **G7. Las teclas de confirmación no siguen el idioma**: muestra «[y]»
  aunque `s`, `o` o `j` también funcionan (`messages.go:522`,
  `tui.go:855`).
  **Hecho:** la tecla de «sí» sigue el idioma; `y` vale siempre.
- [x] **G8. La tarjeta de confirmación tiene el mismo borde para cualquier
  riesgo** (`tui.go:44`, `:971-979`).
  - Debe: acento para normal, aviso con «⚠» para dangerous y error para
    blocked.
  **Hecho:** borde por riesgo: acento, aviso con ⚠ y error.

### H. Funcionamiento

- [x] **H1. Enter aprueba un comando peligroso** (`tui.go:855`), y la
  tarjeta no lo advierte.
  - Debe: con riesgo Dangerous, Enter no hace nada y se pide `y`; la línea
    de teclas lo dice.
  **Hecho:** Enter no aprueba un comando peligroso y la tarjeta lo dice.
- [x] **H2. ctrl+c en una sesión sale de golpe**, a mitad de un
  `brew upgrade` (`tui.go:776-778`). El piloto y la guía piden dos pulsaciones.
  - Debe: la primera cancela la herramienta o el turno; la segunda sale.
  **Hecho:** el primer ctrl+c para solo el turno con `Agent.CancelTurn()` y la sesión queda esperando; el segundo sale (`internal/agent/cancel_test.go`).
- [x] **H3. En terminales estrechos los mínimos fijos desbordan**:
  `picker.go:165` (40), `menu.go:250`, `pilot.go:851`, `guide.go:653` (60) y
  `field()` (34).
  - Debe: `min` en vez de `max`, y un modo compacto por debajo de ~50
    columnas.
  - El editor de `custom` ya se adapta.
  **Hecho:** sin mínimos que desborden y modo compacto por debajo de 50 columnas.
- [x] **H4. Las pistas de teclas se cortan en silencio por la derecha**
  (`tui.go:1126-1156`, `menu.go:342`, `guide.go:706`, `:776`). En el menú
  se pierde «q salir».
  - Debe: un `hintLine(w, …)` que reparta las pistas en líneas o quite
    primero las menos importantes. El editor de `custom` usa ya
    `wrapHints`, que se puede reutilizar.
  **Hecho:** `hintLine` quita primero las pistas opcionales y nunca pierde ayuda y salir.
- [x] **H5. Con poco ancho, la barra de estado tira toda la parte derecha**:
  modelo, tokens, coste y tiempo (`tui.go:1099-1102`).
  - Debe: quitar las partes una a una.
  **Hecho:** la barra de estado se degrada por pasos y conserva coste y tiempo.
- [x] **H6. El picker ignora el alto del terminal** (`pickerRows = 10`), y
  el plan del recorrido guiado no tiene scroll (`guide.go:651-707`).
  **Hecho:** picker con `min(10, alto-8)` filas; el plan hace scroll.
- [x] **H7. No se avisa de las líneas nuevas cuando el usuario ha subido**
  (`tui.go:577`), ni hay `home`/`end`/`G` en la transcripción.
  - Debe: un aviso «↓ N líneas nuevas · End».
  **Hecho:** «↓ N líneas nuevas · end», y home/end/g/G.
- [x] **H8. Faltan indicadores de «hay más».** El picker muestra «… N» solo
  por abajo, el menú no indica nada y el autocompletado se corta en 6 sin
  avisar.
  **Hecho:** indicadores «↑/↓ N más» en picker, menú, autocompletado y plan.
- [x] **H9. Los badges del picker no están alineados** (`picker.go:212-220`,
  `model.png`).
  - Hay que regenerar `card_choice*.snap`.
  **Hecho:** badges en columna y snapshots regeneradas.
- [x] **H10. Un paso largo y callado no se distingue de uno colgado.**
  - Debe: «sin salida desde 45 s» a partir de unos 30 s (`tui.go:483-487`).
  **Hecho:** «sin salida desde 45 s» a partir de 30 s.
- [x] **H11. La bienvenida repite lo que ya muestra el panel** (tarea, modo,
  modelo) y su caja de 78 columnas no se alinea con él.
  - El modo, además, aparece cortado: «asks before risky ac…».
  **Hecho:** bienvenida compacta con el panel visible y etiquetas cortas de modo.
- [x] **H12. El ratón captura la selección de texto** (`WithMouseCellMotion`)
  y no se dice cómo seleccionar.
  - Debe: «shift+arrastrar» en la ayuda y `OMEGA_MOUSE=0`.
  **Hecho:** ayuda con «shift+arrastrar» y `OMEGA_MOUSE=0`.
- [x] **H13. `--no-tui` no se activa con TERM=dumb en la sesión**
  (`root.go:630`), aunque la guía sí lo comprueba (`full.go:437`).
  - `setup` solo comprueba stdin, así que con stdout redirigido escribe
    escapes en el fichero.
  - Debe: una sola función `wantTUI()`.
  **Hecho:** `ui.WantTUI` usado en la sesión, la guía y `setup`.
- [x] **H14. No hay forma de reducir la animación** (shimmer a 10 fps,
  mascota, spinner).
  - Debe: `OMEGA_REDUCED_MOTION`.
  - De paso, sacar los estilos que `shimmer` crea en cada fotograma.
  **Hecho:** `OMEGA_REDUCED_MOTION=1` o `NO_COLOR`; estilos del shimmer fuera del fotograma.
- [x] **H15. Con la búsqueda abierta, la transcripción se pinta tres veces
  por refresco** (`tui.go:563`, `:570`, `:1180`).
  - Debe: cachear mientras no cambie.
  **Hecho:** caché de la transcripción y de la búsqueda.
- [x] **H16. El resumen de la guía recorta en vez de ajustar**
  (`guide.go:295`, `:875`), y pinta en rojo las notas de etapas que no
  fallaron.
  **Hecho:** las notas se ajustan al ancho y solo se pintan en rojo las etapas que fallaron o se pararon.
- [x] **H17. La pantalla de arranque («starting…») no tiene estilo**
  (`tui.go:1202`).
  **Hecho:** pantalla de arranque con marca y spinner centrados.

---

## Nuevas funciones

- [x] **N1. `omega custom`, siguientes pasos.**
  - Varias rutinas con nombre: `omega custom semanal`, `--set semanal`, y
    elegir una por defecto.
  - Programarla: `omega schedule install --task custom`. La rutina ya es
    determinista y sin IA, así que encaja con lo desatendido.
  - Opciones por paso en el editor:
    - «solo si hay menos de X GB libres»;
    - «parar si este falla»;
    - «saltar los lunes».
  - Exportarla e importarla con `export` y `bootstrap`.
  - Mostrarla y editarla en la app de escritorio, a través de
    `internal/cli/guiapi.go`.
  - Al recomendar, enviar también datos agregados y no personales si el
    usuario lo acepta: espacio libre, días desde la última ejecución, qué
    pasos fallaron.
  - Que el editor muestre cuánto tardó y cuánto liberó cada paso en las
    últimas ejecuciones.
  **Hecho:** rutinas con nombre (`--list`, `--default`), `stop_on_fail` (tecla `f`), `only_below_free_gb` (tecla `b`), días de la semana (`--days`, tecla `w`), duración y espacio liberado de la última vez en el editor, tarea programada `custom`, `export`/`bootstrap` llevan `custom.toml`, datos del equipo opcionales para la recomendación (tecla `d`, apagado por defecto) y tarjeta de rutinas en la app de escritorio, probada en la app real (Wails en modo servidor, con sus bindings): reordenar, parar si falla y días quedan en `custom.toml` igual que los escribe `omega custom --set`.
- [x] **N2. `omega steps show <id>`**: comando completo, qué hace, riesgo,
  instalación y últimas ejecuciones. Ver F21.
  **Hecho:** `omega steps show <id> [--json]`.
- [x] **N3. `omega history`**: una vista de logs, pending y notes en una
  pantalla, con filtro por tarea y por fecha. Ver F6.
  **Hecho:** `omega history` con `-n`, `--task`, `--since` y `--json`.
- [x] **N4. `omega notes compact`**: propone fusionar las notas repetidas y
  borrar las obsoletas, con una tarjeta para aprobarlo. Ver D6.
  **Hecho:** `omega notes compact [--dry-run] [-y]`, sin IA y con copia `notes.md.bak`.
- [x] **N5. `omega logs`**: añadir `--json`, `-n N`, `--task`, `--failed`, y
  `logs view --tools`, que muestre solo las llamadas y sus resultados.
  **Hecho:** `logs --json`, `-n`, `--task`, `--failed` y `logs view --tools`.

---

## Mantenibilidad y pruebas

- [x] **M1. Ficheros demasiado grandes.**
  - `safety/safety.go` (1470 líneas), `ui/tui.go` (1260), `agent/tools.go`
    (1240), `ui/pilot.go` (1020) y `safety/secret.go` (920).
  - Las pruebas se organizan por ronda de revisión (`review3_test.go`,
    `review4_test.go`) y no por funcionalidad.
  **Hecho:** `safety.go`, `tools.go`, `tui.go` y `pilot.go` repartidos en ficheros por tema; las pruebas nuevas van en ficheros por funcionalidad. Los `review3/review4_test.go` antiguos no se renombran.
- [x] **M2. Lógica duplicada.**
  - `shellQuote` está copiada seis veces (agent, runner, sudo, macos,
    gitops, scheduler).
  - `pause()`, en anthropic y en openai.
  - `wrappers` y `commandSegments` del agente (`tools.go:355-363`, `:697`)
    rehacen `safety.afterWrappers` y `shellSegments`, con un split que
    ignora las comillas.
  - Hay `regexp.MustCompile` dentro de funciones que se llaman mucho
    (`tools.go:357`, `:681`).
  **Hecho:** `safety.ShellQuote`/`runner.ShellQuote`, `safety.Segments` y `safety.AfterWrappers` exportados y usados; `pause()` en un sitio; regexp fuera de las funciones calientes.
- [x] **M3. Código muerto**: `scheduler.Notify` (`scheduler.go:103-126`) no
  se llama desde ningún sitio.
  **Hecho:** borrado `scheduler.Notify`.
- [x] **M4. Recortes que parten UTF-8** en `runner.capture.add`
  (`runner.go:705`, `:712`) y en `fetch_url` (`web.go:292`).
  - `maxNote` cuenta bytes: 400 bytes son menos de 400 caracteres en
    español.
  - `pending` ya está arreglado en esta pasada.
  **Hecho:** recortes por runas en runner, `fetch_url`, `run_history` y pendientes; `maxNote` cuenta runas.
- [x] **M5. Claves que chocan en el registro**: `callKey = tool+command`
  (`log.go:265`). Dos llamadas iguales en el mismo lote se pisan.
  **Hecho:** cada llamada lleva su propio id en el registro.
- [x] **M6. Falsos positivos de la política** que obligan a aprobar
  lecturas: `crontab -l`, `gh auth status` y `env FOO=1 brew upgrade` salen
  Dangerous.
  - Varios pasos Safe (`topgrade-preview`, `mole-*-preview`, `brew-doctor`,
    `dev-artifacts`) se clasifican Dangerous, así que nunca entran en el
    lote paralelo.
  **Hecho:** `crontab -l` y `gh auth status` son Safe; `env X=1 cmd` se juzga por el comando; los pasos integrados Safe entran en el lote paralelo.
- [x] **M7. El ayudante de sudo tiene rutas fijas** (`#!/bin/bash`,
  `/usr/bin/sudo`, `internal/sudo/sudo.go:40-51`) y falla en NixOS.
  **Hecho:** `#!/usr/bin/env bash` y `sudo` buscado en el PATH.
- [x] **M8. El registro del programador no rota.**
  - `schedule.log` y `schedule.err` crecen sin límite.
  - A la unidad de systemd le faltan `Nice` e `IOSchedulingClass=idle`, y al
    plist `ProcessType=Background`.
  **Hecho:** `scheduler.RotateLogs()`, `Nice`/`IOSchedulingClass=idle` en systemd, `ProcessType=Background` en launchd. schtasks no admite prioridad con `/TR`.
- [x] **M9. La caché web no se poda nunca**
  (`~/.local/state/omega/web-cache`).
  - Además `fetch_url` pasa al modelo contenido no HTML (PDF, binarios)
    tal cual.
  **Hecho:** la caché se poda a 24 h y `fetch_url` rechaza lo que no es texto.
- [x] **M10. Faltan pruebas de regresión** para:
  - A1-A10;
  - el entorno del programador en Windows;
  - el código de salida de `brew` y de `disk_usage`;
  - la invalidación de la caché tras `write_file`;
  - el formato de los `%` en las traducciones (E7).
  **Hecho:** pruebas para A1–A10, el programador por sistema, `brew` con `pipefail`, `disk_usage`, la caché, los verbos de las traducciones, `CancelTurn` y `--max-cost`.
- [x] **M11. Las snapshots de la TUI solo cubren cuatro tarjetas** y no ven
  el color.
  - Debe: añadir snapshots del menú, el panel, la guía, el editor de
    `custom` y los informes de texto (`report.go`) a 60, 80 y 120 columnas.
  **Hecho:** snapshots del menú, la sesión, el plan, el editor de rutinas y `report.go` a 60, 80 y 120 columnas (`go test ./internal/ui -run Snap -update`).

---

## Cómo comprobarlo

- `go test ./...` y `go vet ./...` pasan.
- Ninguna de estas órdenes modifica nada; muestran la salida nueva:

  ```bash
  make build
  ./bin/omega steps
  ./bin/omega doctor
  ./bin/omega logs
  ./bin/omega pending
  ./bin/omega notes
  ./bin/omega models
  ```

- Para probar `custom` sin tocar la configuración real:

  ```bash
  XDG_CONFIG_HOME=$(mktemp -d) ./bin/omega custom --set
  ```

  Dentro del editor:
  - espacio, ⇧↑/⇧↓ y enter para montar y guardar la rutina;
  - `i` para pedir la recomendación (necesita una clave configurada);
  - después, `omega custom --show` y `omega custom --dry-run`.
- Sin color: `NO_COLOR=1 ./bin/omega steps` y `./bin/omega logs | cat` no
  deben tener escapes.
- Para los P0 de la política: cada caso de A1-A5 es una entrada de tabla
  para `safety.Classify` y `safety.AlwaysConfirm`, con `want always=true`.
