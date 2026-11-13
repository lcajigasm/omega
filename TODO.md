# TODO: P0 de omega (CLI y app de escritorio)

Revisión del 13 de noviembre de 2026 sobre `main` en `1069103` (v0.24.0).

Un P0 es un defecto que, en un uso normal, borra datos del usuario, deja
ejecutar un comando sin la aprobación que omega promete, filtra un secreto,
o deja una función principal rota. Lo que solo es mejorable no está aquí.

Cada punto dice qué pasaba y qué tiene que pasar. Se marca cuando el
cambio está hecho. Todos tienen una prueba salvo D8 y D9 (Windows y el
`Info.plist` de la app, que no se pueden ejecutar en las pruebas), D10 (se
comprobó en la app en modo servidor) y D11.

## A. Pérdida de datos

- [x] **A1. `omega disk artifacts` borra ramas de git y carpetas de código.**
  Cualquier carpeta llamada `build`, `target`, `venv`… cuenta como artefacto,
  también dentro de `.git` (la rama `build/x` es la carpeta
  `.git/refs/heads/build`), y la antigüedad se mide con la fecha de la
  carpeta, no con la del proyecto: un `node_modules` instalado hace meses en
  un proyecto que se edita hoy sale como «inactivo».
  Debe: no entrar en `.git`; exigir la señal del proyecto (`package.json`,
  `Cargo.toml`, `pyvenv.cfg`…); saltar lo que git tiene versionado; medir la
  última actividad del proyecto.
- [x] **A2. `omega uninstall <nombre>` busca por subcadena y borra para siempre.**
  `uninstall Code` selecciona los datos de Xcode; `uninstall ""` selecciona
  todo `~/Library/Application Support`, `Caches` y `Preferences`.
  Debe: nombre exacto (o el identificador del bundle), rechazar nombres
  vacíos o de un carácter, y mover a la Papelera en vez de borrar.
- [x] **A3. `omega skills update` borra copias editadas y su `.git`.**
  La decisión de conservar la carpeta vieja se toma con el hash de una sola
  instalación y se aplica a todas las copias. Además la papelera, en otro
  volumen, copia sin `.git` ni enlaces y borra el original.
  Debe: la carpeta vieja va siempre a la papelera, completa.
- [x] **A4. `omega git gc` ejecuta `gc --prune=now --aggressive` en todos los repos.**
  Borra al momento lo recuperable (stash descartado, commits de una rama
  borrada) y git lo documenta como inseguro con otro proceso escribiendo.
  Debe: `git gc` normal (dos semanas de gracia) y saltar repos con bloqueo.
- [x] **A5. El modo sin IA ignora «confirmar siempre» con `--yes`.**
  `omega clean --yes --no-ai` (y la caída automática a modo determinista
  cuando no hay clave, que es lo que le pasa a una tarea programada) ejecuta
  `docker volume prune -a -f` sin preguntar. `omega shell` lo hace con
  `yes` fijo.
  Debe: esos pasos solo corren con una confirmación en un terminal; sin
  terminal se saltan y se dice.
- [x] **A6. `omega quick --dry-run` vacía la Papelera.**
  No mira `--dry-run`, no pregunta, imprime ✓ aunque el comando falle y
  termina siempre con «tu equipo está sano».
  Debe: respetar `--dry-run`, enseñar el resultado real y devolver error.
- [x] **A7. `omega analyze --no-ai` («sin cambiar nada») arranca Docker.**
  El filtro es solo la categoría; `docker-start` es una comprobación
  peligrosa. Debe: en `analyze` y `health` solo pasos de solo lectura.
- [x] **A8. `omega mac undo` dice «deshecho» sin restaurar.**
  Si el destino ya existe, `mv -n` no mueve (o anida) y sale con 0.
  Debe: fallar cuando el destino existe.

## B. Comandos que se ejecutan sin la aprobación prometida

- [x] **B1. El clasificador no entiende `\"` ni `$'…'`.**
  `echo \"; rm -rf ~/Documents; echo \"` se clasifica como solo lectura y
  se ejecuta sin preguntar. Debe: tratar el escape como el shell.
- [x] **B2. Formas de zsh que ejecutan comandos:** `=(cmd)` y el
  calificador de glob `*(e:'cmd':)`. Debe: peligroso.
- [x] **B3. Prefijos «de solo lectura» que escriben o ejecutan:**
  `rg --pre`, `sort -o`, `uniq entrada salida`, `env -S`, `find -fprint`,
  `git branch -D`, `git log --output`, `go env -w`, `brew tap usuario/repo`,
  y cualquier `VAR=valor` delante (`PATH=/tmp/x ls`, `GIT_CONFIG_*`).
  Debe: peligroso.
- [x] **B4. `systemctl -p status poweroff`:** el valor de una opción se toma
  por el subcomando. Debe: una opción desconocida antes del subcomando es
  peligrosa.
- [x] **B5. PowerShell:** `Get-ChildItem | ForEach-Object { Remove-Item … }`
  y `Write-Output (Remove-Item …)` pasan como solo lectura.
  Debe: un bloque `{}`, un paréntesis o una llamada a método es peligroso.

## C. Secretos

- [x] **C1. La contraseña de sudo guardada se entrega a cualquier proceso.**
  Con `--sudo` o `unattended`, un comando puede pedirla al socket y
  escribirla en la salida, que va al modelo y al registro.
  Debe: bloquear los comandos que nombran `SUDO_ASKPASS` o el socket, y
  sustituir la contraseña por `[redacted]` en toda salida.
- [x] **C2. Huecos en la lectura de secretos:** globs (`cat ~/.s*/id_*`),
  rutas relativas (`cat .ssh/clave`), `~/.git-credentials`,
  `~/.docker/config.json`, credenciales de gcloud, `security … -wa`, y
  variables como `GITHUB_TOKEN` que llegan al modelo con `env`.
  Debe: bloqueado, y esas variables fuera del entorno de los comandos.
- [x] **C3. `fetch_url` llega a `localhost`, a la red local y a
  `169.254.169.254` sin preguntar.** Debe: rechazar direcciones privadas,
  también tras una redirección.
- [x] **C4. `read_file` sobre `/dev/zero` o una tubería** agota la memoria o
  se queda colgado. Debe: solo ficheros normales y lectura con límite.
- [x] **C5. Los registros de sesión son legibles por todos** (`0644` en una
  carpeta `0755`) y la copia de seguridad de un fichero `0600` queda `0644`.
  Debe: `0600`/`0700` y el modo del original.
- [x] **C6. App: una imagen remota en el texto del modelo se descarga sola**
  (canal de exfiltración sin clic). Debe: una CSP que solo deje imágenes
  propias.

## D. Funciones rotas

- [x] **D1. Sesión inservible tras el límite de turnos o un corte por
  `max_tokens`:** queda un `tool_use` sin resultado en el historial y el
  proveedor responde 400 a todo lo siguiente; sin terminal, un corte sale
  con 0. Debe: historial válido y salida distinta de 0.
- [x] **D2. `omega shell`: `audit`, `net` y `privacy` terminan en pánico**, y
  `net ping` vacía la caché DNS (índices sobre una lista ordenada).
  Debe: buscar el subcomando por nombre y pasarle un comando real.
- [x] **D3. `--json` no es JSON en modo sin IA:** `update --json` imprime
  texto antes del objeto, `--format json` no imprime JSON, `--fail-on-*` se
  ignora con `--json`, y una ejecución en la que todo falló sale con 0.
  Debe: solo JSON en stdout, códigos de salida después, error si algo falla.
- [x] **D4. `omega --no-ai` (la tarea por defecto) no existe:** «task 'full'
  not supported». Debe: ejecutar actualización, limpieza y análisis.
- [x] **D5. La interfaz sin TUI se cuelga tras Ctrl+C en la pregunta de
  seguimiento.** Debe: terminar.
- [x] **D6. `omega schedule install` acepta cualquier cosa y dice
  «instalado»:** `--interval monthly` instala semanal, `--task bogus` o
  `--hour 99` instalan una tarea que falla siempre, y en Windows la tarea
  no puede arrancar (comillas de `cmd` dentro de PowerShell).
  Debe: validar, y `monthly` de verdad.
- [x] **D7. La salida de un comando se pierde entera si una línea pasa de
  1 MB.** Debe: cortar la línea y seguir.
- [x] **D8. App en Windows: cada comando abre una ventana de consola.**
  Debe: `CREATE_NO_WINDOW`.
- [x] **D9. App en macOS: falta `NSAppleEventsUsageDescription`,** así que
  quitar un elemento de inicio o cerrar una app de la barra de menús falla
  sin diálogo. Debe: estar en `Info.plist`.
- [x] **D10. App: la tarjeta de aprobación nace con el foco en «Aprobar»**
  (un espacio o Enter aprueba un comando peligroso) y «Borrar notas» borra
  con un clic. Debe: foco en «Denegar» y dos clics para borrar.
- [x] **D11. App: `Start` mantiene el cerrojo mientras consulta la red,** y
  con red lenta `Stop` y el cierre de la ventana esperan. Debe: reservar y
  soltar.

## Fuera de esta lista (no son P0)

- El `xattr` del cask hace falta tras cada `brew upgrade --cask`, no «una vez».
- Los trabajos `image` y `publish` del release no dependen uno del otro.
- `steps --json` y `doctor --json` no existen.
- No hay compactación del historial de la conversación.
- En Windows, cancelar mata PowerShell pero no a sus hijos.
- `(sleep 30 &); echo` se informa como fallido.
- La clave de API pasa por argv de `security add-generic-password`.
