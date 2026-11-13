# TODO: segunda revisión de omega (CLI y app de escritorio)

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
