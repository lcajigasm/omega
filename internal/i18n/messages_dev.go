package i18n

// Messages of omega dev, the privacy and apps sections of omega mac, the
// session panel and the home screen. Same order as messages: en, es, fr, de, pt.
var devMessages = map[string][]string{
	// ---- omega mac permissions / apps ----
	"cmd.mac_permissions": {
		"Remove privacy permissions left behind by binaries and apps that are gone",
		"Quita los permisos de privacidad que dejaron binarios y apps que ya no existen",
		"Retirer les autorisations de confidentialité laissées par des binaires et apps disparus",
		"Datenschutz-Berechtigungen entfernen, die verschwundene Programme und Apps hinterlassen haben",
		"Remove as permissões de privacidade deixadas por binários e apps que já não existem",
	},
	"cmd.mac_apps": {
		"Review apps you no longer open, Intel-only apps and what removed apps left behind",
		"Revisa las apps que ya no abres, las que solo son Intel y los restos de apps eliminadas",
		"Passer en revue les apps que vous n'ouvrez plus, les apps Intel et les restes d'apps supprimées",
		"Apps prüfen, die du nicht mehr öffnest, reine Intel-Apps und Reste entfernter Apps",
		"Revê as apps que já não abres, as que são só Intel e os restos de apps removidas",
	},
	"mac.no_tcc": {
		"Your privacy database could not be read: only system-wide permissions are listed. Give your terminal Full Disk Access (System Settings → Privacy & Security → Full Disk Access) to see the rest.",
		"No se ha podido leer tu base de datos de privacidad: solo se listan los permisos de todo el sistema. Da Acceso total al disco a tu terminal (Ajustes del Sistema → Privacidad y seguridad → Acceso total al disco) para ver el resto.",
		"Votre base de confidentialité n'a pas pu être lue : seules les autorisations système sont listées. Donnez l'accès complet au disque à votre terminal (Réglages Système → Confidentialité et sécurité → Accès complet au disque) pour voir le reste.",
		"Deine Datenschutz-Datenbank konnte nicht gelesen werden: Nur systemweite Berechtigungen werden angezeigt. Gib deinem Terminal Festplattenvollzugriff (Systemeinstellungen → Datenschutz & Sicherheit → Festplattenvollzugriff), um den Rest zu sehen.",
		"Não foi possível ler a tua base de dados de privacidade: só aparecem as permissões de todo o sistema. Dá Acesso total ao disco ao teu terminal (Definições do Sistema → Privacidade e segurança → Acesso total ao disco) para ver o resto.",
	},
	"mac.h.permissions": {"PRIVACY PERMISSIONS", "PERMISOS DE PRIVACIDAD", "AUTORISATIONS DE CONFIDENTIALITÉ", "DATENSCHUTZ-BERECHTIGUNGEN", "PERMISSÕES DE PRIVACIDADE"},
	"mac.h.apps":        {"APPS TO REVIEW", "APPS PARA REVISAR", "APPS À PASSER EN REVUE", "APPS ZUM PRÜFEN", "APPS PARA REVER"},
	"mac.h.leftovers":   {"LEFTOVERS OF REMOVED APPS", "RESTOS DE APPS ELIMINADAS", "RESTES D'APPS SUPPRIMÉES", "RESTE ENTFERNTER APPS", "RESTOS DE APPS REMOVIDAS"},
	"mac.perm_summary": {
		"%d entries, %d left over",
		"%d entradas, %d sobran",
		"%d entrées, %d en trop",
		"%d Einträge, %d davon übrig geblieben",
		"%d entradas, %d a mais",
	},
	"mac.perm_gone": {"the binary no longer exists", "el binario ya no existe", "le binaire n'existe plus", "Programm existiert nicht mehr", "o binário já não existe"},
	"mac.intel":     {"Intel only (Rosetta)", "solo Intel (Rosetta)", "Intel uniquement (Rosetta)", "nur Intel (Rosetta)", "só Intel (Rosetta)"},
	"mac.pick.permissions": {
		"Which privacy entries should be removed?",
		"¿Qué entradas de privacidad se quitan?",
		"Quelles entrées de confidentialité retirer ?",
		"Welche Datenschutz-Einträge sollen entfernt werden?",
		"Que entradas de privacidade se removem?",
	},
	"mac.pick.apps": {
		"What should go to the Trash?",
		"¿Qué va a la Papelera?",
		"Que faut-il mettre à la corbeille ?",
		"Was soll in den Papierkorb?",
		"O que vai para o Lixo?",
	},
	"mac.leftover_places": {"in %d places of ~/Library", "en %d sitios de ~/Library", "à %d endroits de ~/Library", "an %d Stellen in ~/Library", "em %d sítios de ~/Library"},
	"mac.leftover_vendor": {
		"another app of the same vendor is installed",
		"hay instalada otra app del mismo fabricante",
		"une autre app du même éditeur est installée",
		"eine andere App desselben Herstellers ist installiert",
		"há outra app do mesmo fabricante instalada",
	},
	"mac.act.perm_reset": {
		"Remove %s from %s",
		"Quitar %s de %s",
		"Retirer %s de %s",
		"%s aus %s entfernen",
		"Remover %s de %s",
	},
	"mac.act.perm_reset_all": {
		"Forget %s for every app (they will ask again)",
		"Olvidar %s para todas las apps (volverán a preguntar)",
		"Oublier %s pour toutes les apps (elles redemanderont)",
		"%s für alle Apps vergessen (sie fragen erneut)",
		"Esquecer %s para todas as apps (voltarão a perguntar)",
	},
	"mac.act.app_remove": {"Uninstall %s", "Desinstalar %s", "Désinstaller %s", "%s deinstallieren", "Desinstalar %s"},
	"mac.act.leftover_trash": {
		"Move the leftovers of %s to the Trash (%s)",
		"Mover los restos de %s a la Papelera (%s)",
		"Mettre les restes de %s à la corbeille (%s)",
		"Reste von %s in den Papierkorb legen (%s)",
		"Mover os restos de %s para o Lixo (%s)",
	},
	"mac.perm_still": {
		"macOS did not let omega remove %s from %s (system-wide permissions can only be changed in System Settings). The pane is open: select the entry and press –.",
		"macOS no ha dejado a omega quitar %s de %s (los permisos de todo el sistema solo se cambian en Ajustes del Sistema). El panel está abierto: selecciona la entrada y pulsa –.",
		"macOS n'a pas laissé omega retirer %s de %s (les autorisations système ne se changent que dans Réglages Système). Le panneau est ouvert : sélectionnez l'entrée et appuyez sur –.",
		"macOS hat omega nicht erlaubt, %s aus %s zu entfernen (systemweite Berechtigungen lassen sich nur in den Systemeinstellungen ändern). Der Bereich ist geöffnet: Eintrag auswählen und – drücken.",
		"O macOS não deixou o omega remover %s de %s (as permissões de todo o sistema só se mudam nas Definições do Sistema). O painel está aberto: seleciona a entrada e carrega em –.",
	},
	"mac.perm_deleted": {
		"removed from your privacy database (copy of the database before the change: %s)",
		"quitado de tu base de datos de privacidad (copia de la base de datos antes del cambio: %s)",
		"retiré de votre base de confidentialité (copie de la base avant le changement : %s)",
		"aus deiner Datenschutz-Datenbank entfernt (Kopie der Datenbank vor der Änderung: %s)",
		"removido da tua base de dados de privacidade (cópia da base de dados antes da alteração: %s)",
	},
	"mac.perm_unverified": {
		"tccutil reported success, but the privacy database cannot be read from here to confirm it.",
		"tccutil ha informado de éxito, pero desde aquí no se puede leer la base de datos de privacidad para confirmarlo.",
		"tccutil indique un succès, mais la base de confidentialité ne peut pas être lue d'ici pour le confirmer.",
		"tccutil meldet Erfolg, aber die Datenschutz-Datenbank lässt sich von hier nicht lesen, um es zu bestätigen.",
		"O tccutil indicou sucesso, mas daqui não se consegue ler a base de dados de privacidade para o confirmar.",
	},

	// ---- omega dev ----
	"cmd.dev": {
		"Review, install and tune your languages and command-line developer tools (AI-guided)",
		"Revisa, instala y optimiza tus lenguajes y herramientas de desarrollo de consola (guiado por IA)",
		"Passe en revue, installe et optimise vos langages et outils de développement en ligne de commande (guidé par IA)",
		"Prüft, installiert und optimiert deine Sprachen und Entwickler-Werkzeuge für die Kommandozeile (KI-geführt)",
		"Revê, instala e otimiza as tuas linguagens e ferramentas de desenvolvimento de consola (guiado por IA)",
	},
	"cmd.dev_long": {
		`Without a subcommand, the AI reviews your development setup: version managers that overlap, commands
shadowed on PATH, runtimes past their end of life, old versions piling up, configuration worth tuning,
stale build folders and tools you are missing. It asks on cards and applies only what you pick.

  omega dev               guided review with AI (--auto applies the low-risk suggestions itself)
  omega dev status        what is installed, by which manager, and what stands out (no AI)
  omega dev install       pick tools to install from omega's catalogue (or name them: omega dev install fzf mise)
  omega dev activate      add the missing shell lines of installed tools to ~/.zshrc or ~/.bashrc`,
		`Sin subcomando, la IA revisa tu entorno de desarrollo: gestores de versiones que se solapan, comandos
tapados en el PATH, runtimes fuera de soporte, versiones antiguas acumuladas, configuración mejorable,
carpetas de compilación olvidadas y herramientas que te faltan. Pregunta con tarjetas y aplica solo lo que elijas.

  omega dev               revisión guiada con IA (--auto aplica por sí misma lo de bajo riesgo)
  omega dev status        qué hay instalado, con qué gestor, y qué llama la atención (sin IA)
  omega dev install       elige herramientas del catálogo de omega (o nómbralas: omega dev install fzf mise)
  omega dev activate      añade a ~/.zshrc o ~/.bashrc las líneas que les faltan a las herramientas instaladas`,
		`Sans sous-commande, l'IA passe en revue votre environnement de développement : gestionnaires de versions qui se
chevauchent, commandes masquées dans le PATH, runtimes en fin de vie, anciennes versions accumulées, configuration
à améliorer, dossiers de build oubliés et outils manquants. Elle demande sur des cartes et n'applique que vos choix.

  omega dev               revue guidée par IA (--auto applique elle-même ce qui est peu risqué)
  omega dev status        ce qui est installé, par quel gestionnaire, et ce qui ressort (sans IA)
  omega dev install       choisir des outils du catalogue d'omega (ou les nommer : omega dev install fzf mise)
  omega dev activate      ajouter à ~/.zshrc ou ~/.bashrc les lignes manquantes des outils installés`,
		`Ohne Unterbefehl prüft die KI deine Entwicklungsumgebung: sich überschneidende Versionsmanager, im PATH
verdeckte Befehle, Laufzeiten nach Support-Ende, angesammelte alte Versionen, verbesserbare Konfiguration,
vergessene Build-Ordner und fehlende Werkzeuge. Sie fragt auf Karten und wendet nur an, was du auswählst.

  omega dev               geführte Prüfung mit KI (--auto wendet Risikoarmes selbst an)
  omega dev status        was installiert ist, von welchem Manager, und was auffällt (ohne KI)
  omega dev install       Werkzeuge aus omegas Katalog auswählen (oder nennen: omega dev install fzf mise)
  omega dev activate      fehlende Shell-Zeilen installierter Werkzeuge in ~/.zshrc oder ~/.bashrc eintragen`,
		`Sem subcomando, a IA revê o teu ambiente de desenvolvimento: gestores de versões que se sobrepõem, comandos
tapados no PATH, runtimes fora de suporte, versões antigas acumuladas, configuração a melhorar,
pastas de compilação esquecidas e ferramentas em falta. Pergunta com cartões e aplica só o que escolheres.

  omega dev               revisão guiada com IA (--auto aplica por si o que é de baixo risco)
  omega dev status        o que está instalado, por que gestor, e o que chama a atenção (sem IA)
  omega dev install       escolhe ferramentas do catálogo do omega (ou indica-as: omega dev install fzf mise)
  omega dev activate      acrescenta a ~/.zshrc ou ~/.bashrc as linhas em falta das ferramentas instaladas`,
	},
	"cmd.dev_status": {
		"Installed languages and tools, what installed each one, and what stands out (no AI)",
		"Lenguajes y herramientas instalados, quién instaló cada uno y qué llama la atención (sin IA)",
		"Langages et outils installés, ce qui a installé chacun et ce qui ressort (sans IA)",
		"Installierte Sprachen und Werkzeuge, wer sie installiert hat und was auffällt (ohne KI)",
		"Linguagens e ferramentas instaladas, quem instalou cada uma e o que chama a atenção (sem IA)",
	},
	"cmd.dev_install": {
		"Install developer tools from omega's catalogue",
		"Instala herramientas de desarrollo del catálogo de omega",
		"Installer des outils de développement du catalogue d'omega",
		"Entwickler-Werkzeuge aus omegas Katalog installieren",
		"Instala ferramentas de desenvolvimento do catálogo do omega",
	},
	"cmd.dev_activate": {
		"Add the shell lines installed tools still need to ~/.zshrc or ~/.bashrc",
		"Añade a ~/.zshrc o ~/.bashrc las líneas que aún necesitan las herramientas instaladas",
		"Ajouter à ~/.zshrc ou ~/.bashrc les lignes dont les outils installés ont encore besoin",
		"Shell-Zeilen, die installierte Werkzeuge noch brauchen, in ~/.zshrc oder ~/.bashrc eintragen",
		"Acrescenta a ~/.zshrc ou ~/.bashrc as linhas de que as ferramentas instaladas ainda precisam",
	},
	"dev.reading": {
		"Looking at your development setup…",
		"Mirando tu entorno de desarrollo…",
		"Examen de votre environnement de développement…",
		"Deine Entwicklungsumgebung wird untersucht…",
		"A ver o teu ambiente de desenvolvimento…",
	},
	"dev.cat.lang":      {"LANGUAGES", "LENGUAJES", "LANGAGES", "SPRACHEN", "LINGUAGENS"},
	"dev.cat.versions":  {"VERSION MANAGERS", "GESTORES DE VERSIONES", "GESTIONNAIRES DE VERSIONS", "VERSIONSMANAGER", "GESTORES DE VERSÕES"},
	"dev.cat.packages":  {"PACKAGES AND BUILDS", "PAQUETES Y COMPILACIÓN", "PAQUETS ET BUILDS", "PAKETE UND BUILDS", "PACOTES E COMPILAÇÃO"},
	"dev.cat.shell":     {"SHELL AND TERMINAL", "SHELL Y TERMINAL", "SHELL ET TERMINAL", "SHELL UND TERMINAL", "SHELL E TERMINAL"},
	"dev.cat.git":       {"GIT", "GIT", "GIT", "GIT", "GIT"},
	"dev.cat.container": {"CONTAINERS AND CLOUD", "CONTENEDORES Y NUBE", "CONTENEURS ET CLOUD", "CONTAINER UND CLOUD", "CONTENTORES E NUVEM"},
	"dev.cat.ai":        {"AI ASSISTANTS", "ASISTENTES DE IA", "ASSISTANTS IA", "KI-ASSISTENTEN", "ASSISTENTES DE IA"},
	"dev.inactive":      {"not activated in %s", "sin activar en %s", "non activé dans %s", "nicht in %s aktiviert", "por ativar em %s"},
	"dev.h.findings":    {"FINDINGS", "HALLAZGOS", "CONSTATS", "BEFUNDE", "ACHADOS"},
	"dev.h.versions":    {"INSTALLED VERSIONS", "VERSIONES INSTALADAS", "VERSIONS INSTALLÉES", "INSTALLIERTE VERSIONEN", "VERSÕES INSTALADAS"},
	"dev.no_findings":   {"Nothing stands out.", "Nada llama la atención.", "Rien à signaler.", "Nichts Auffälliges.", "Nada chama a atenção."},
	"dev.conflict": {
		"%s versions come from several managers: %s",
		"las versiones de %s vienen de varios gestores: %s",
		"les versions de %s viennent de plusieurs gestionnaires : %s",
		"%s-Versionen stammen von mehreren Managern: %s",
		"as versões de %s vêm de vários gestores: %s",
	},
	"dev.shadow": {
		"%s is on PATH several times; the first one runs: %s",
		"%s está varias veces en el PATH; se ejecuta el primero: %s",
		"%s est plusieurs fois dans le PATH ; le premier s'exécute : %s",
		"%s liegt mehrfach im PATH; der erste wird ausgeführt: %s",
		"%s está várias vezes no PATH; corre o primeiro: %s",
	},
	"dev.path_dup":  {"PATH repeats: %s", "el PATH repite: %s", "le PATH répète : %s", "PATH wiederholt: %s", "o PATH repete: %s"},
	"dev.path_gone": {"PATH has folders that do not exist: %s", "el PATH tiene carpetas que no existen: %s", "le PATH contient des dossiers inexistants : %s", "PATH enthält Ordner, die es nicht gibt: %s", "o PATH tem pastas que não existem: %s"},
	"dev.not_active": {
		"%s is installed but not activated: add  %s  to %s (omega dev activate)",
		"%s está instalado pero sin activar: añade  %s  a %s (omega dev activate)",
		"%s est installé mais pas activé : ajoutez  %s  à %s (omega dev activate)",
		"%s ist installiert, aber nicht aktiviert: trage  %s  in %s ein (omega dev activate)",
		"%s está instalado mas por ativar: acrescenta  %s  a %s (omega dev activate)",
	},
	"dev.startup": {
		"an interactive %s starts in %d ms",
		"un %s interactivo arranca en %d ms",
		"un %s interactif démarre en %d ms",
		"eine interaktive %s startet in %d ms",
		"um %s interativo arranca em %d ms",
	},
	"dev.status_next": {
		"%d catalogue tools are not installed: omega dev install. Let the AI review all of this with omega dev.",
		"%d herramientas del catálogo no están instaladas: omega dev install. Deja que la IA revise todo esto con omega dev.",
		"%d outils du catalogue ne sont pas installés : omega dev install. Laissez l'IA revoir tout cela avec omega dev.",
		"%d Katalog-Werkzeuge sind nicht installiert: omega dev install. Lass die KI das alles mit omega dev prüfen.",
		"%d ferramentas do catálogo não estão instaladas: omega dev install. Deixa a IA rever tudo isto com omega dev.",
	},
	"dev.all_installed": {
		"Everything in omega's catalogue is already installed.",
		"Todo lo del catálogo de omega ya está instalado.",
		"Tout le catalogue d'omega est déjà installé.",
		"Alles aus omegas Katalog ist bereits installiert.",
		"Tudo o que está no catálogo do omega já está instalado.",
	},
	"dev.pick.install":  {"What should be installed?", "¿Qué se instala?", "Que faut-il installer ?", "Was soll installiert werden?", "O que se instala?"},
	"dev.pick.activate": {"Add these lines to %s?", "¿Añadir estas líneas a %s?", "Ajouter ces lignes à %s ?", "Diese Zeilen in %s eintragen?", "Acrescentar estas linhas a %s?"},
	"dev.unknown": {
		"omega's catalogue has no tool %q; see omega dev install",
		"el catálogo de omega no tiene la herramienta %q; mira omega dev install",
		"le catalogue d'omega n'a pas d'outil %q ; voir omega dev install",
		"omegas Katalog kennt kein Werkzeug %q; siehe omega dev install",
		"o catálogo do omega não tem a ferramenta %q; vê omega dev install",
	},
	"dev.no_install": {
		"omega detects %s but does not install it",
		"omega detecta %s pero no lo instala",
		"omega détecte %s mais ne l'installe pas",
		"omega erkennt %s, installiert es aber nicht",
		"o omega deteta %s mas não o instala",
	},
	"dev.already": {"%s is already installed.", "%s ya está instalado.", "%s est déjà installé.", "%s ist bereits installiert.", "%s já está instalado."},
	"dev.need_brew": {
		"These tools are installed with Homebrew, which is not on this Mac. Install it first: https://brew.sh",
		"Estas herramientas se instalan con Homebrew, que no está en este Mac. Instálalo primero: https://brew.sh",
		"Ces outils s'installent avec Homebrew, absent de ce Mac. Installez-le d'abord : https://brew.sh",
		"Diese Werkzeuge werden mit Homebrew installiert, das auf diesem Mac fehlt. Installiere es zuerst: https://brew.sh",
		"Estas ferramentas instalam-se com o Homebrew, que não está neste Mac. Instala-o primeiro: https://brew.sh",
	},
	"dev.activate_other": {
		"omega edits the startup file of zsh and bash only. Add these lines to your shell's startup file (adapting zsh to your shell):",
		"omega solo edita el archivo de inicio de zsh y bash. Añade estas líneas al de tu shell (cambiando zsh por tu shell):",
		"omega ne modifie que le fichier de démarrage de zsh et de bash. Ajoutez ces lignes à celui de votre shell (en adaptant zsh à votre shell) :",
		"omega bearbeitet nur die Startdatei von zsh und bash. Trage diese Zeilen in die deiner Shell ein (zsh an deine Shell anpassen):",
		"O omega só edita o ficheiro de arranque do zsh e do bash. Acrescenta estas linhas ao da tua shell (trocando zsh pela tua shell):",
	},
	"dev.activated": {
		"Added to %s (copy of the previous file: %s). Open a new terminal to use them.",
		"Añadido a %s (copia del archivo anterior: %s). Abre un terminal nuevo para usarlas.",
		"Ajouté à %s (copie de l'ancien fichier : %s). Ouvrez un nouveau terminal pour les utiliser.",
		"In %s eingetragen (Kopie der alten Datei: %s). Öffne ein neues Terminal, um sie zu nutzen.",
		"Acrescentado a %s (cópia do ficheiro anterior: %s). Abre um terminal novo para as usar.",
	},
	"dev.activated_new": {
		"Created %s. Open a new terminal to use them.",
		"Creado %s. Abre un terminal nuevo para usarlas.",
		"%s créé. Ouvrez un nouveau terminal pour les utiliser.",
		"%s angelegt. Öffne ein neues Terminal, um sie zu nutzen.",
		"Criado %s. Abre um terminal novo para as usar.",
	},

	// ---- interface: slash commands, session panel, home screen ----
	"slash.panel": {"Show or hide the session panel", "Muestra u oculta el panel de sesión", "Affiche ou masque le panneau de session", "Sitzungsleiste ein- oder ausblenden", "Mostra ou esconde o painel da sessão"},
	"slash.tasks": {"Start another task in this session", "Empieza otra tarea en esta sesión", "Lance une autre tâche dans cette session", "Startet eine weitere Aufgabe in dieser Sitzung", "Começa outra tarefa nesta sessão"},
	"slash.task_full": {
		"Full maintenance: check, update everything, clean up and summarise",
		"Mantenimiento completo: comprobar, actualizar todo, limpiar y resumir",
		"Maintenance complète : vérifier, tout mettre à jour, nettoyer et résumer",
		"Komplette Wartung: prüfen, alles aktualisieren, aufräumen und zusammenfassen",
		"Manutenção completa: verificar, atualizar tudo, limpar e resumir",
	},
	"slash.task_update":      {"Update all installed software", "Actualiza todo el software instalado", "Met à jour tous les logiciels installés", "Aktualisiert die gesamte installierte Software", "Atualiza todo o software instalado"},
	"slash.task_clean":       {"Free up disk space, biggest wins first", "Libera espacio en disco, primero lo que más ocupa", "Libère de l'espace disque, le plus gros d'abord", "Gibt Speicherplatz frei, das Größte zuerst", "Liberta espaço em disco, primeiro o que mais ocupa"},
	"slash.task_analyze":     {"Look at the state of this Mac without changing anything", "Analiza el estado de este Mac sin cambiar nada", "Analyse l'état de ce Mac sans rien changer", "Untersucht den Zustand dieses Macs, ohne etwas zu ändern", "Analisa o estado deste Mac sem mudar nada"},
	"slash.task_tune":        {"Review and improve the configuration of your tools", "Revisa y mejora la configuración de tus herramientas", "Revoit et améliore la configuration de vos outils", "Prüft und verbessert die Konfiguration deiner Werkzeuge", "Revê e melhora a configuração das tuas ferramentas"},
	"slash.task_fix":         {"Diagnose and repair topgrade and Mole when they fail", "Diagnostica y repara topgrade y Mole cuando fallan", "Diagnostique et répare topgrade et Mole quand ils échouent", "Diagnostiziert und repariert topgrade und Mole, wenn sie scheitern", "Diagnostica e repara o topgrade e o Mole quando falham"},
	"slash.task_mac":         {"Review login items, background jobs, menu bar, Dock, shortcuts, permissions and apps", "Revisa inicio de sesión, segundo plano, barra de menús, Dock, atajos, permisos y apps", "Revoit ouverture, arrière-plan, barre des menus, Dock, raccourcis, autorisations et apps", "Prüft Anmeldeobjekte, Hintergrund, Menüleiste, Dock, Kürzel, Berechtigungen und Apps", "Revê início de sessão, segundo plano, barra de menus, Dock, atalhos, permissões e apps"},
	"slash.task_permissions": {"Clean up privacy permissions of binaries and apps that are gone", "Limpia los permisos de privacidad de binarios y apps que ya no existen", "Nettoie les autorisations de binaires et d'apps disparus", "Räumt Berechtigungen verschwundener Programme und Apps auf", "Limpa as permissões de binários e apps que já não existem"},
	"slash.task_apps":        {"Review unused apps, Intel-only apps and leftovers", "Revisa apps sin usar, apps solo Intel y restos", "Revoit les apps inutilisées, les apps Intel et les restes", "Prüft ungenutzte Apps, reine Intel-Apps und Reste", "Revê apps sem uso, apps só Intel e restos"},
	"slash.task_dev":         {"Review languages, version managers and developer tools", "Revisa lenguajes, gestores de versiones y herramientas de desarrollo", "Revoit langages, gestionnaires de versions et outils de développement", "Prüft Sprachen, Versionsmanager und Entwickler-Werkzeuge", "Revê linguagens, gestores de versões e ferramentas de desenvolvimento"},

	"slash.task_install": {"Pick developer tools to install, with the AI's advice", "Elige herramientas de desarrollo para instalar, con el consejo de la IA", "Choisir des outils de développement à installer, avec les conseils de l'IA", "Entwickler-Werkzeuge zum Installieren auswählen, mit Rat der KI", "Escolhe ferramentas de desenvolvimento para instalar, com o conselho da IA"},
	"slash.task_docker":  {"Free up the space Docker uses, only what you pick", "Libera el espacio que usa Docker, solo lo que elijas", "Libère l'espace utilisé par Docker, seulement ce que vous choisissez", "Gibt von Docker belegten Platz frei, nur was du auswählst", "Liberta o espaço que o Docker usa, só o que escolheres"},
	"slash.task_undo":    {"See what omega changed on this Mac and revert what you pick", "Mira qué cambió omega en este Mac y deshaz lo que elijas", "Voir ce qu'omega a changé sur ce Mac et annuler ce que vous choisissez", "Zeigt, was omega auf diesem Mac geändert hat, und macht Ausgewähltes rückgängig", "Vê o que o omega mudou neste Mac e desfaz o que escolheres"},
	"slash.task_history": {"What failed or was left undone in earlier runs", "Qué falló o quedó sin hacer en ejecuciones anteriores", "Ce qui a échoué ou est resté à faire lors des exécutions précédentes", "Was in früheren Läufen scheiterte oder liegen blieb", "O que falhou ou ficou por fazer em execuções anteriores"},
	"slash.task_doctor":  {"Check that omega itself is set up correctly", "Comprueba que omega está bien configurado", "Vérifie qu'omega est correctement configuré", "Prüft, ob omega selbst richtig eingerichtet ist", "Verifica se o omega está bem configurado"},
	"agent.dev_activate": {
		"Add these lines to %s so the tools work in new terminals. The file is copied first.",
		"Añadir estas líneas a %s para que las herramientas funcionen en terminales nuevos. Antes se copia el archivo.",
		"Ajouter ces lignes à %s pour que les outils fonctionnent dans les nouveaux terminaux. Le fichier est copié d'abord.",
		"Diese Zeilen in %s eintragen, damit die Werkzeuge in neuen Terminals funktionieren. Die Datei wird vorher kopiert.",
		"Acrescentar estas linhas a %s para que as ferramentas funcionem em terminais novos. O ficheiro é copiado antes.",
	},
	"agent.notes_forget": {
		"Delete these notes from omega's memory:",
		"Borrar estas notas de la memoria de omega:",
		"Supprimer ces notes de la mémoire d'omega :",
		"Diese Notizen aus omegas Gedächtnis löschen:",
		"Apagar estas notas da memória do omega:",
	},
	"agent.notes_forgot": {"%d notes deleted", "%d notas borradas", "%d notes supprimées", "%d Notizen gelöscht", "%d notas apagadas"},
	"agent.settings_set": {
		"Change omega's configuration (%s)",
		"Cambiar la configuración de omega (%s)",
		"Modifier la configuration d'omega (%s)",
		"omegas Konfiguration ändern (%s)",
		"Alterar a configuração do omega (%s)",
	},

	"panel.session":  {"SESSION", "SESIÓN", "SESSION", "SITZUNG", "SESSÃO"},
	"panel.mode":     {"mode", "modo", "mode", "Modus", "modo"},
	"panel.steps":    {"steps", "pasos", "étapes", "Schritte", "passos"},
	"panel.disk":     {"DISK", "DISCO", "DISQUE", "SPEICHER", "DISCO"},
	"panel.free":     {"%s free of %s", "%s libres de %s", "%s libres sur %s", "%s frei von %s", "%s livres de %s"},
	"panel.freed":    {"%s freed", "%s liberados", "%s libérés", "%s freigegeben", "%s libertados"},
	"panel.used":     {"%s more in use", "%s más en uso", "%s de plus utilisés", "%s mehr belegt", "%s a mais em uso"},
	"panel.usage":    {"TOKENS", "TOKENS", "TOKENS", "TOKENS", "TOKENS"},
	"panel.activity": {"ACTIVITY", "ACTIVIDAD", "ACTIVITÉ", "AKTIVITÄT", "ATIVIDADE"},
	"panel.none":     {"nothing yet", "nada todavía", "rien pour l'instant", "noch nichts", "nada ainda"},

	"cmd.menu": {
		"Home screen: everything omega can do, started with Enter",
		"Pantalla de inicio: todo lo que omega sabe hacer, a un Enter",
		"Écran d'accueil : tout ce qu'omega sait faire, lancé avec Entrée",
		"Startbildschirm: alles, was omega kann, mit Enter gestartet",
		"Ecrã inicial: tudo o que o omega sabe fazer, à distância de um Enter",
	},
	"menu.g.maintain": {"MAINTENANCE", "MANTENIMIENTO", "MAINTENANCE", "WARTUNG", "MANUTENÇÃO"},
	"menu.g.mac":      {"YOUR MAC", "TU MAC", "VOTRE MAC", "DEIN MAC", "O TEU MAC"},
	"menu.g.dev":      {"DEVELOPMENT", "DESARROLLO", "DÉVELOPPEMENT", "ENTWICKLUNG", "DESENVOLVIMENTO"},
	"menu.g.setup":    {"OMEGA", "OMEGA", "OMEGA", "OMEGA", "OMEGA"},
	"menu.last_run":   {"last run", "última", "dernière", "zuletzt", "última"},
	"menu.disk":       {"disk", "disco", "disque", "Speicher", "disco"},
	"menu.ago_min":    {"%d min ago", "hace %d min", "il y a %d min", "vor %d Min.", "há %d min"},
	"menu.ago_hours":  {"%d hours ago", "hace %d horas", "il y a %d heures", "vor %d Stunden", "há %d horas"},
	"menu.ago_days":   {"%d days ago", "hace %d días", "il y a %d jours", "vor %d Tagen", "há %d dias"},
	"menu.no_match":   {"Nothing matches %q", "Nada coincide con %q", "Rien ne correspond à %q", "Nichts passt zu %q", "Nada corresponde a %q"},
	"menu.hint": {
		"↑↓ move · enter run · type to filter · q quit",
		"↑↓ mover · enter ejecutar · escribe para filtrar · q salir",
		"↑↓ déplacer · entrée lancer · tapez pour filtrer · q quitter",
		"↑↓ bewegen · Enter starten · tippen zum Filtern · q beenden",
		"↑↓ mover · enter executar · escreve para filtrar · q sair",
	},
	"menu.hint_filter": {"enter run · esc clear", "enter ejecutar · esc borrar", "entrée lancer · échap effacer", "Enter starten · Esc löschen", "enter executar · esc limpar"},
	"menu.no_key": {
		"No API key for the configured provider yet: run omega setup",
		"Aún no hay clave de API para el proveedor configurado: ejecuta omega setup",
		"Pas encore de clé d'API pour le fournisseur configuré : lancez omega setup",
		"Noch kein API-Schlüssel für den eingestellten Anbieter: omega setup ausführen",
		"Ainda não há chave de API para o fornecedor configurado: corre omega setup",
	},
	"menu.need_tty": {
		"omega menu needs a terminal; in scripts, call the commands directly",
		"omega menu necesita un terminal; en scripts, llama a los comandos directamente",
		"omega menu a besoin d'un terminal ; dans un script, appelez les commandes directement",
		"omega menu braucht ein Terminal; in Skripten die Befehle direkt aufrufen",
		"o omega menu precisa de um terminal; em scripts, chama os comandos diretamente",
	},
	"cmd.models_long": {
		"Lists the provider's models with their benchmark score and list price, best first, and marks the two that automatic mode uses.",
		"Lista los modelos del proveedor con su nota en pruebas de rendimiento y su precio, de mejor a peor, y marca los dos que usa el modo automático.",
		"Liste les modèles du fournisseur avec leur score et leur prix, du meilleur au moins bon, et marque les deux qu'utilise le mode automatique.",
		"Listet die Modelle des Anbieters mit Benchmark-Wert und Listenpreis, die besten zuerst, und markiert die zwei des automatischen Modus.",
		"Lista os modelos do fornecedor com a nota em testes de desempenho e o preço, do melhor para o pior, e marca os dois que o modo automático usa.",
	},
	"menu.back": {
		"Enter returns to the menu · ctrl+c leaves ",
		"Enter vuelve al menú · ctrl+c sale ",
		"Entrée revient au menu · ctrl+c quitte ",
		"Enter zurück zum Menü · Strg+C beendet ",
		"Enter volta ao menu · ctrl+c sai ",
	},

	// ---- model catalogue ----
	"models.col_model": {"model", "modelo", "modèle", "Modell", "modelo"},
	"models.col_score": {"score", "nota", "score", "Wert", "nota"},
	"models.col_in":    {"in $/M", "entrada $/M", "entrée $/M", "Eingabe $/M", "entrada $/M"},
	"models.col_out":   {"out $/M", "salida $/M", "sortie $/M", "Ausgabe $/M", "saída $/M"},
	"models.mark_fast": {"fast", "rápido", "rapide", "schnell", "rápido"},
	"models.mark_smart": {
		"smart", "potente", "puissant", "stark", "potente",
	},
	"models.mark_both": {"fast and smart", "rápido y potente", "rapide et puissant", "schnell und stark", "rápido e potente"},
	"models.estimated": {
		"~ score estimated from other benchmarks; not used for the automatic choice while measured ones exist",
		"~ nota estimada a partir de otras pruebas; no se usa en la elección automática mientras haya notas medidas",
		"~ score estimé à partir d'autres tests ; ignoré par le choix automatique tant qu'il existe des scores mesurés",
		"~ Wert aus anderen Benchmarks geschätzt; für die automatische Wahl nicht benutzt, solange gemessene vorliegen",
		"~ nota estimada a partir de outros testes; não é usada na escolha automática enquanto houver notas medidas",
	},
	"models.source": {
		"Prices and scores: OpenRouter's public catalogue (Artificial Analysis index), read %s.",
		"Precios y notas: catálogo público de OpenRouter (índice de Artificial Analysis), leído el %s.",
		"Prix et scores : catalogue public d'OpenRouter (indice Artificial Analysis), lu le %s.",
		"Preise und Werte: öffentlicher Katalog von OpenRouter (Artificial-Analysis-Index), gelesen am %s.",
		"Preços e notas: catálogo público da OpenRouter (índice Artificial Analysis), lido em %s.",
	},
	"models.none": {
		"No model catalogue yet: automatic mode uses the names built into omega.",
		"Aún no hay catálogo de modelos: el modo automático usa los nombres incluidos en omega.",
		"Pas encore de catalogue de modèles : le mode automatique utilise les noms intégrés à omega.",
		"Noch kein Modellkatalog: der automatische Modus benutzt die in omega eingebauten Namen.",
		"Ainda não há catálogo de modelos: o modo automático usa os nomes incluídos no omega.",
	},
	"models.off": {
		"The model catalogue is off (model_catalog = \"off\"): automatic mode uses the names built into omega.",
		"El catálogo de modelos está desactivado (model_catalog = \"off\"): el modo automático usa los nombres incluidos en omega.",
		"Le catalogue de modèles est désactivé (model_catalog = \"off\") : le mode automatique utilise les noms intégrés à omega.",
		"Der Modellkatalog ist aus (model_catalog = \"off\"): der automatische Modus benutzt die in omega eingebauten Namen.",
		"O catálogo de modelos está desativado (model_catalog = \"off\"): o modo automático usa os nomes incluídos no omega.",
	},
	"models.auto": {
		"Automatic mode: %s for routine work, %s when a step fails and for fix.",
		"Modo automático: %s para lo rutinario, %s cuando un paso falla y en fix.",
		"Mode automatique : %s pour la routine, %s quand une étape échoue et pour fix.",
		"Automatischer Modus: %s für Routine, %s wenn ein Schritt scheitert und für fix.",
		"Modo automático: %s para o rotineiro, %s quando um passo falha e em fix.",
	},
	"models.fixed": {
		"The model is fixed to %s. Automatic mode would use %s and %s (omega model auto).",
		"El modelo está fijado en %s. El modo automático usaría %s y %s (omega model auto).",
		"Le modèle est fixé sur %s. Le mode automatique utiliserait %s et %s (omega model auto).",
		"Das Modell ist auf %s festgelegt. Der automatische Modus würde %s und %s benutzen (omega model auto).",
		"O modelo está fixado em %s. O modo automático usaria %s e %s (omega model auto).",
	},
	"models.since": {
		"These two have been the choice since %s.", "Estos dos son la elección desde el %s.", "Ces deux là sont le choix depuis le %s.",
		"Diese beiden sind die Wahl seit dem %s.", "Estes dois são a escolha desde %s.",
	},
	"models.fetch_failed": {
		"could not refresh the model catalogue: %v", "no se pudo actualizar el catálogo de modelos: %v",
		"impossible d'actualiser le catalogue de modèles : %v", "Modellkatalog konnte nicht aktualisiert werden: %v",
		"não foi possível atualizar o catálogo de modelos: %v",
	},
	"flag.refresh": {
		"download the model catalogue again and choose again", "descargar de nuevo el catálogo de modelos y volver a elegir",
		"télécharger à nouveau le catalogue de modèles et choisir à nouveau", "Modellkatalog neu laden und neu wählen",
		"descarregar de novo o catálogo de modelos e voltar a escolher",
	},
	"config.bad_catalog": {
		"model_catalog must be live or off", "model_catalog debe ser live u off", "model_catalog doit être live ou off",
		"model_catalog muss live oder off sein", "model_catalog deve ser live ou off",
	},
	"doctor.models_auto": {
		"models: automatic, %s → %s (catalogue of %s)", "modelos: automático, %s → %s (catálogo del %s)",
		"modèles : automatique, %s → %s (catalogue du %s)", "Modelle: automatisch, %s → %s (Katalog vom %s)",
		"modelos: automático, %s → %s (catálogo de %s)",
	},
	"doctor.models_builtin": {
		"models: automatic, %s → %s (built-in names; no catalogue)", "modelos: automático, %s → %s (nombres incluidos; sin catálogo)",
		"modèles : automatique, %s → %s (noms intégrés ; pas de catalogue)", "Modelle: automatisch, %s → %s (eingebaute Namen; kein Katalog)",
		"modelos: automático, %s → %s (nomes incluídos; sem catálogo)",
	},
}

func init() {
	for k, v := range devMessages {
		messages[k] = v
	}
}
