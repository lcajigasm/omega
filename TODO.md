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

- [ ] **Modo automático y código opaco.** Con `--yes`, `… | base64 -d | sh`,
  `sh fichero`, `python3 -c …` o `docker run -v ~:/h …` solo son
  «peligrosos» y se ejecutan; lo mismo `gh auth token`, `gcloud auth
  print-access-token`, `security -i`. Hace falta una segunda clase de
  «preguntar siempre». La clave del proveedor se retira por nombre pero su
  valor no está registrado en la máscara.
- [ ] **sudo.** Un ayudante falso puesto con `SUDO_ASKPASS` bajo un sudo
  real aún puede recibir la contraseña; requiere un comando con `sudo`
  aprobado.
- [ ] **Borrados del directorio personal no bloqueados:** `rm -rf ~usuario`,
  `rm -rf $PWD`, `rm -rf /Users/l*`, `mv ~ /tmp/x`, `rsync --delete … ~/`,
  `git -C ~ clean -fdx`, y herramientas de disco que no están en la lista
  (`mke2fs`, `sgdisk --zap-all`, `shred /dev/…`, `asr restore --erase`).
- [ ] **«Preguntar siempre» no reconoce** `podman system reset`,
  `docker compose down --volumes=true`, `docker.exe …`, `docker service rm`.
- [ ] **Windows:** `ipconfig /release`, `net use * /delete`, `net start`,
  una ruta UNC (`dir \\host\share` envía el hash NTLM) y familias enteras
  de verbos `Get-…` son de solo lectura; `Get-ChildItem ~\.ssh |
  Get-Content` lee las claves.
- [ ] **git en carpetas no fiables:** filtros `clean`/`smudge`,
  `diff.external` y `textconv` del `.git/config` del repositorio.
- [ ] **`write_file`:** la tarjeta de un fichero nuevo no enseña su
  contenido; faltan en «preguntar siempre» `~/.aws/config`
  (`credential_process`), `~/.kube/config`, ejecutables existentes.
  `~/.gitconfig` queda fuera a propósito.
- [ ] **`mac_policy` cambia `mac.toml` sin aprobación** y `mac_change
  disable:` es «normal».
- [ ] **`propose_actions` levanta el modo de solo lectura** con cualquier
  elección, y en una ejecución desatendida se eligen todas.
- [ ] **`omega uninstall Code`** (sin app con ese nombre) ofrece los datos
  de Visual Studio Code.
- [ ] **`fetch_url`:** rangos que el filtro no cubre (NAT64, 6to4,
  `198.18.0.0/15`); con un proxy solo queda la comprobación previa.
- [ ] **Comandos de solo lectura que no terminan:** `mdfind -live`,
  `tail --foll`, `pmset -g pslog`.
- [ ] **Ficheros y límites:** `config.toml` y `mac.toml` son legibles por
  otros usuarios; las respuestas del proveedor se leen sin límite; no hay
  tope de llamadas por turno; la unidad de systemd no escapa `%` ni `$`.
- [ ] **App:** `RunTool` no se puede cancelar; `Browser.OpenURL` acepta
  esquemas que no son http(s) si los pide un script de la página.
- [ ] **Publicación:** fijar por commit las acciones `docker/*`, añadir
  atestación de procedencia a los archivos, hash SRI al reproductor de
  asciinema de la web; `omega net audit` imprime las credenciales de un
  proxy.

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
