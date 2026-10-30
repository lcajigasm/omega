package i18n

// Messages of omega mac: login and background items, menu bar, Dock and
// keyboard shortcuts. Same order as messages: en, es, fr, de, pt.
var macMessages = map[string][]string{
	"cmd.mac": {
		"Control what opens at login, runs in the background, sits in the menu bar or the Dock, and global keyboard shortcuts (AI-guided)",
		"Controla lo que se abre al iniciar sesión, corre en segundo plano, está en la barra de menús o en el Dock, y los atajos de teclado globales (guiado por IA)",
		"Contrôle ce qui s'ouvre à la connexion, tourne en arrière-plan, occupe la barre des menus ou le Dock, et les raccourcis clavier globaux (guidé par IA)",
		"Steuert, was beim Anmelden startet, im Hintergrund läuft, in der Menüleiste oder im Dock sitzt, und globale Tastenkürzel (KI-geführt)",
		"Controla o que abre no início de sessão, corre em segundo plano, fica na barra de menus ou no Dock, e os atalhos de teclado globais (guiado por IA)",
	},
	"cmd.mac_long": {
		`Without a subcommand, the AI reviews everything with your rules (omega mac policy) and omega's defaults,
asks you on cards and applies only what you pick. The subcommands do the same for one area without AI.

  omega mac               guided review with AI (--auto applies the low-risk suggestions itself)
  omega mac status        overview, changes nothing
  omega mac login         apps that open at login
  omega mac background    jobs allowed in the background
  omega mac menubar       menu bar apps
  omega mac dock          the Dock
  omega mac shortcuts     who keeps each contested keyboard shortcut
  omega mac policy        your rules and omega's defaults
  omega mac undo          revert changes`,
		`Sin subcomando, la IA revisa todo con tus reglas (omega mac policy) y los valores por defecto de omega,
te pregunta con tarjetas y aplica solo lo que elijas. Los subcomandos hacen lo mismo para un área, sin IA.

  omega mac               revisión guiada con IA (--auto aplica por sí misma lo de bajo riesgo)
  omega mac status        resumen, no cambia nada
  omega mac login         apps que se abren al iniciar sesión
  omega mac background    procesos permitidos en segundo plano
  omega mac menubar       apps de la barra de menús
  omega mac dock          el Dock
  omega mac shortcuts     quién se queda cada atajo de teclado en conflicto
  omega mac policy        tus reglas y los valores por defecto de omega
  omega mac undo          deshacer cambios`,
		`Sans sous-commande, l'IA passe tout en revue avec vos règles (omega mac policy) et les valeurs par défaut d'omega,
vous demande sur des cartes et n'applique que ce que vous choisissez. Les sous-commandes font de même pour un domaine, sans IA.

  omega mac               revue guidée par IA (--auto applique lui-même ce qui est peu risqué)
  omega mac status        vue d'ensemble, ne change rien
  omega mac login         apps ouvertes à la connexion
  omega mac background    tâches autorisées en arrière-plan
  omega mac menubar       apps de la barre des menus
  omega mac dock          le Dock
  omega mac shortcuts     qui garde chaque raccourci clavier en conflit
  omega mac policy        vos règles et les valeurs par défaut d'omega
  omega mac undo          annuler des changements`,
		`Ohne Unterbefehl prüft die KI alles mit deinen Regeln (omega mac policy) und omegas Standardwerten,
fragt dich über Karten und wendet nur an, was du auswählst. Die Unterbefehle tun dasselbe für einen Bereich, ohne KI.

  omega mac               geführte Prüfung mit KI (--auto wendet risikoarme Vorschläge selbst an)
  omega mac status        Übersicht, ändert nichts
  omega mac login         Apps, die beim Anmelden starten
  omega mac background    im Hintergrund erlaubte Prozesse
  omega mac menubar       Apps in der Menüleiste
  omega mac dock          das Dock
  omega mac shortcuts     wer welches umkämpfte Tastenkürzel behält
  omega mac policy        deine Regeln und omegas Standardwerte
  omega mac undo          Änderungen rückgängig machen`,
		`Sem subcomando, a IA revê tudo com as tuas regras (omega mac policy) e os valores por omissão do omega,
pergunta-te com cartões e aplica só o que escolheres. Os subcomandos fazem o mesmo para uma área, sem IA.

  omega mac               revisão guiada com IA (--auto aplica sozinha o que é de baixo risco)
  omega mac status        resumo, não muda nada
  omega mac login         apps que abrem no início de sessão
  omega mac background    processos permitidos em segundo plano
  omega mac menubar       apps da barra de menus
  omega mac dock          o Dock
  omega mac shortcuts     quem fica com cada atalho de teclado em conflito
  omega mac policy        as tuas regras e os valores por omissão do omega
  omega mac undo          desfazer alterações`,
	},
	"cmd.mac_status":      {"Overview of all areas, changes nothing", "Resumen de todas las áreas, no cambia nada", "Vue d'ensemble, ne change rien", "Übersicht aller Bereiche, ändert nichts", "Resumo de todas as áreas, não muda nada"},
	"cmd.mac_login":       {"Choose which apps open at login", "Elige qué apps se abren al iniciar sesión", "Choisir les apps ouvertes à la connexion", "Wählen, welche Apps beim Anmelden starten", "Escolher que apps abrem no início de sessão"},
	"cmd.mac_background":  {"Choose what may run in the background", "Elige qué puede ejecutarse en segundo plano", "Choisir ce qui peut tourner en arrière-plan", "Wählen, was im Hintergrund laufen darf", "Escolher o que pode correr em segundo plano"},
	"cmd.mac_menubar":     {"Review the apps in the menu bar", "Revisa las apps de la barra de menús", "Passer en revue les apps de la barre des menus", "Apps in der Menüleiste prüfen", "Rever as apps da barra de menus"},
	"cmd.mac_dock":        {"Clean up the Dock", "Ordena el Dock", "Ranger le Dock", "Das Dock aufräumen", "Arrumar o Dock"},
	"cmd.mac_shortcuts":   {"Resolve global keyboard shortcut conflicts", "Resuelve conflictos de atajos de teclado globales", "Résoudre les conflits de raccourcis clavier globaux", "Konflikte globaler Tastenkürzel lösen", "Resolver conflitos de atalhos de teclado globais"},
	"cmd.mac_policy":      {"Show your rules and omega's defaults", "Muestra tus reglas y los valores por defecto de omega", "Afficher vos règles et les valeurs par défaut d'omega", "Deine Regeln und omegas Standardwerte zeigen", "Mostrar as tuas regras e os valores por omissão do omega"},
	"cmd.mac_policy_edit": {"Edit your rules (mac.toml)", "Edita tus reglas (mac.toml)", "Modifier vos règles (mac.toml)", "Deine Regeln bearbeiten (mac.toml)", "Editar as tuas regras (mac.toml)"},
	"cmd.mac_undo":        {"Revert changes made by omega mac", "Deshace cambios hechos por omega mac", "Annuler des changements faits par omega mac", "Änderungen von omega mac rückgängig machen", "Desfazer alterações feitas pelo omega mac"},

	"agent.mac_rule": {"rule saved (%s): %s", "regla guardada (%s): %s", "règle enregistrée (%s) : %s", "Regel gespeichert (%s): %s", "regra guardada (%s): %s"},

	"mac.reading": {
		"Reading what runs on your Mac… (macOS can take up to a minute to list login and background items)",
		"Leyendo lo que se ejecuta en tu Mac… (macOS puede tardar hasta un minuto en listar los elementos de inicio y segundo plano)",
		"Lecture de ce qui tourne sur votre Mac… (macOS peut mettre jusqu'à une minute à lister les éléments d'ouverture et d'arrière-plan)",
		"Lese, was auf deinem Mac läuft… (macOS braucht bis zu einer Minute, um Anmelde- und Hintergrundobjekte aufzulisten)",
		"A ler o que corre no teu Mac… (o macOS pode demorar até um minuto a listar os itens de início e segundo plano)",
	},
	"mac.no_btm": {
		"macOS did not list the login items in time; only launchd jobs are shown. Try again in a moment.",
		"macOS no listó a tiempo los elementos de inicio; solo se muestran los trabajos de launchd. Inténtalo de nuevo en un momento.",
		"macOS n'a pas listé les éléments d'ouverture à temps ; seules les tâches launchd sont affichées. Réessayez dans un instant.",
		"macOS hat die Anmeldeobjekte nicht rechtzeitig geliefert; nur launchd-Jobs werden gezeigt. Versuch es gleich noch einmal.",
		"O macOS não listou a tempo os itens de início; só são mostrados os trabalhos do launchd. Tenta de novo daqui a pouco.",
	},
	"mac.h.login":      {"OPEN AT LOGIN", "SE ABREN AL INICIAR SESIÓN", "OUVERTS À LA CONNEXION", "BEIM ANMELDEN GEÖFFNET", "ABREM NO INÍCIO DE SESSÃO"},
	"mac.h.background": {"ALLOWED IN THE BACKGROUND", "PERMITIDOS EN SEGUNDO PLANO", "AUTORISÉS EN ARRIÈRE-PLAN", "IM HINTERGRUND ERLAUBT", "PERMITIDOS EM SEGUNDO PLANO"},
	"mac.h.menubar":    {"MENU BAR", "BARRA DE MENÚS", "BARRE DES MENUS", "MENÜLEISTE", "BARRA DE MENUS"},
	"mac.h.dock":       {"DOCK", "DOCK", "DOCK", "DOCK", "DOCK"},
	"mac.h.shortcuts":  {"KEYBOARD SHORTCUTS", "ATAJOS DE TECLADO", "RACCOURCIS CLAVIER", "TASTENKÜRZEL", "ATALHOS DE TECLADO"},

	"mac.verdict.keep":    {"keep", "mantener", "garder", "behalten", "manter"},
	"mac.verdict.disable": {"turn off", "desactivar", "désactiver", "ausschalten", "desativar"},
	"mac.verdict.review":  {"review", "revisar", "à revoir", "prüfen", "rever"},

	"mac.cat.updater":   {"updater", "actualizador", "programme de mise à jour", "Updater", "atualizador"},
	"mac.cat.essential": {"essential", "esencial", "essentiel", "wichtig", "essencial"},
	"mac.cat.on-demand": {"open when needed", "abrir cuando haga falta", "à ouvrir au besoin", "bei Bedarf öffnen", "abrir quando for preciso"},
	"mac.cat.system":    {"Apple", "Apple", "Apple", "Apple", "Apple"},
	"mac.cat.unknown":   {"unknown", "desconocido", "inconnu", "unbekannt", "desconhecido"},

	"mac.kind.login":  {"opens at login", "se abre al iniciar sesión", "s'ouvre à la connexion", "startet beim Anmelden", "abre no início de sessão"},
	"mac.kind.helper": {"app helper", "ayudante de app", "assistant d'app", "App-Helfer", "auxiliar de app"},
	"mac.kind.agent":  {"user agent", "agente de usuario", "agent utilisateur", "Benutzer-Agent", "agente de utilizador"},
	"mac.kind.daemon": {"system daemon", "daemon del sistema", "daemon système", "System-Daemon", "daemon do sistema"},
	"mac.kind.task":   {"background task", "tarea en segundo plano", "tâche d'arrière-plan", "Hintergrundaufgabe", "tarefa em segundo plano"},

	"mac.running": {"running", "en ejecución", "en cours", "läuft", "em execução"},
	"mac.missing": {"program no longer installed", "el programa ya no está instalado", "programme désinstallé", "Programm nicht mehr installiert", "o programa já não está instalado"},
	"mac.sudo":    {"needs admin", "requiere admin", "nécessite admin", "braucht Admin", "requer admin"},
	"mac.off":     {"off", "desactivado", "désactivé", "aus", "desativado"},

	"mac.act.disable":      {"Turn off %s", "Desactivar %s", "Désactiver %s", "%s ausschalten", "Desativar %s"},
	"mac.act.enable":       {"Turn %s back on", "Volver a activar %s", "Réactiver %s", "%s wieder einschalten", "Voltar a ativar %s"},
	"mac.act.login_remove": {"Stop opening %s at login", "Dejar de abrir %s al iniciar sesión", "Ne plus ouvrir %s à la connexion", "%s nicht mehr beim Anmelden öffnen", "Deixar de abrir %s no início de sessão"},
	"mac.act.quit":         {"Quit %s now", "Cerrar %s ahora", "Quitter %s maintenant", "%s jetzt beenden", "Fechar %s agora"},
	"mac.act.hide":         {"Hide the menu bar icon of %s", "Ocultar el icono de %s en la barra de menús", "Masquer l'icône de %s dans la barre des menus", "Menüleisten-Symbol von %s ausblenden", "Ocultar o ícone de %s na barra de menus"},
	"mac.act.hide_icons":   {"Hide menu bar icons (opens the settings)", "Ocultar iconos de la barra de menús (abre los ajustes)", "Masquer des icônes de la barre des menus (ouvre les réglages)", "Menüleisten-Symbole ausblenden (öffnet die Einstellungen)", "Ocultar ícones da barra de menus (abre as definições)"},
	"mac.act.dock_remove":  {"Remove %s from the Dock", "Quitar %s del Dock", "Retirer %s du Dock", "%s aus dem Dock entfernen", "Remover %s do Dock"},
	"mac.act.dock_add":     {"Add %s to the Dock", "Añadir %s al Dock", "Ajouter %s au Dock", "%s zum Dock hinzufügen", "Adicionar %s ao Dock"},
	"mac.act.dock_set":     {"Dock: set %s to %s", "Dock: poner %s en %s", "Dock : régler %s sur %s", "Dock: %s auf %s setzen", "Dock: definir %s como %s"},
	"mac.act.hotkey_off":   {"Turn off %s for %s (%s)", "Desactivar %s en %s (%s)", "Désactiver %s pour %s (%s)", "%s für %s ausschalten (%s)", "Desativar %s em %s (%s)"},
	"mac.act.hotkey_set":   {"%s · %s: %s → %s", "%s · %s: %s → %s", "%s · %s : %s → %s", "%s · %s: %s → %s", "%s · %s: %s → %s"},

	"mac.manual.login": {
		"If %s is still listed, turn it off in System Settings → General → Login Items, or in the app's own settings.",
		"Si %s sigue en la lista, desactívalo en Ajustes del Sistema → General → Ítems de inicio, o en los ajustes de la propia app.",
		"Si %s est toujours listé, désactivez-le dans Réglages Système → Général → Ouverture, ou dans les réglages de l'app.",
		"Falls %s noch aufgeführt ist, schalte es in Systemeinstellungen → Allgemein → Anmeldeobjekte oder in den Einstellungen der App aus.",
		"Se %s continuar na lista, desativa-o em Definições do Sistema → Geral → Elementos de início, ou nas definições da própria app.",
	},
	"mac.manual.background": {
		"To hide %s from the list too, turn it off under Allow in the Background in System Settings → General → Login Items.",
		"Para que %s tampoco aparezca, desactívalo en «Permitir en segundo plano», en Ajustes del Sistema → General → Ítems de inicio.",
		"Pour retirer aussi %s de la liste, désactivez-le sous « Autoriser en arrière-plan » dans Réglages Système → Général → Ouverture.",
		"Damit %s auch nicht mehr erscheint, schalte es unter „Im Hintergrund erlauben“ in Systemeinstellungen → Allgemein → Anmeldeobjekte aus.",
		"Para %s também não aparecer, desativa-o em «Permitir em segundo plano», em Definições do Sistema → Geral → Elementos de início.",
	},
	"mac.manual.menubar": {
		"Turn %s off under Allow in the Menu Bar in System Settings → Menu Bar, or in the app's own settings.",
		"Desactiva %s en «Permitir en la barra de menús», en Ajustes del Sistema → Barra de menús, o en los ajustes de la propia app.",
		"Désactivez %s sous « Autoriser dans la barre des menus » dans Réglages Système → Barre des menus, ou dans les réglages de l'app.",
		"Schalte %s unter „In der Menüleiste erlauben“ in Systemeinstellungen → Menüleiste oder in den Einstellungen der App aus.",
		"Desativa %s em «Permitir na barra de menus», em Definições do Sistema → Barra de menus, ou nas definições da própria app.",
	},
	"mac.manual.menubar_manager": {
		"Hide %s in %s: it keeps the icon out of sight without quitting the app.",
		"Oculta %s en %s: mantiene el icono fuera de la vista sin cerrar la app.",
		"Masquez %s dans %s : l'icône reste cachée sans quitter l'app.",
		"Blende %s in %s aus: Das Symbol verschwindet, ohne die App zu beenden.",
		"Oculta %s no %s: o ícone fica escondido sem fechar a app.",
	},
	"mac.manual.hotkey": {
		"%s keeps this shortcut in its own settings: %s",
		"%s guarda este atajo en sus propios ajustes: %s",
		"%s garde ce raccourci dans ses propres réglages : %s",
		"%s speichert dieses Kürzel in eigenen Einstellungen: %s",
		"%s guarda este atalho nas suas próprias definições: %s",
	},
	"mac.manual.hotkey_to": {
		"%s keeps this shortcut in its own settings (%s): change it to %s.",
		"%s guarda este atajo en sus propios ajustes (%s): cámbialo a %s.",
		"%s garde ce raccourci dans ses propres réglages (%s) : remplacez-le par %s.",
		"%s speichert dieses Kürzel in eigenen Einstellungen (%s): ändere es auf %s.",
		"%s guarda este atalho nas suas próprias definições (%s): muda-o para %s.",
	},

	"mac.pick.login":      {"Which apps should stop opening at login?", "¿Qué apps dejan de abrirse al iniciar sesión?", "Quelles apps ne doivent plus s'ouvrir à la connexion ?", "Welche Apps sollen nicht mehr beim Anmelden starten?", "Que apps deixam de abrir no início de sessão?"},
	"mac.pick.background": {"What should stop running in the background?", "¿Qué deja de ejecutarse en segundo plano?", "Qu'est-ce qui ne doit plus tourner en arrière-plan ?", "Was soll nicht mehr im Hintergrund laufen?", "O que deixa de correr em segundo plano?"},
	"mac.pick.menubar":    {"What should leave the menu bar?", "¿Qué sale de la barra de menús?", "Qu'est-ce qui doit quitter la barre des menus ?", "Was soll aus der Menüleiste verschwinden?", "O que sai da barra de menus?"},
	"mac.pick.dock":       {"What should change in the Dock?", "¿Qué cambia en el Dock?", "Que faut-il changer dans le Dock ?", "Was soll sich im Dock ändern?", "O que muda no Dock?"},
	"mac.pick.shortcut":   {"Who keeps %s?", "¿Quién se queda %s?", "Qui garde %s ?", "Wer behält %s?", "Quem fica com %s?"},
	"mac.pick.changes":    {"Apply these shortcut changes?", "¿Aplicar estos cambios de atajos?", "Appliquer ces changements de raccourcis ?", "Diese Kürzel-Änderungen anwenden?", "Aplicar estas alterações de atalhos?"},
	"mac.pick.undo":       {"Which changes should be reverted?", "¿Qué cambios se deshacen?", "Quels changements annuler ?", "Welche Änderungen rückgängig machen?", "Que alterações desfazer?"},
	"mac.pick_hint": {
		"What your rules and omega's defaults suggest is ticked · space ticks · Enter applies · Esc applies nothing",
		"Está marcado lo que sugieren tus reglas y los valores por defecto de omega · espacio marca · Enter aplica · Esc no aplica nada",
		"Ce que suggèrent vos règles et les valeurs par défaut d'omega est coché · espace coche · Entrée applique · Échap n'applique rien",
		"Was deine Regeln und omegas Standardwerte vorschlagen, ist angehakt · Leertaste hakt an · Enter wendet an · Esc wendet nichts an",
		"Está marcado o que sugerem as tuas regras e os valores por omissão do omega · espaço marca · Enter aplica · Esc não aplica nada",
	},
	"mac.pick_owner_hint": {
		"omega's suggestion comes first · the others move to a free shortcut · your choice is saved for next time",
		"La sugerencia de omega va primero · los demás pasan a un atajo libre · tu elección se guarda para la próxima vez",
		"La suggestion d'omega vient en premier · les autres passent à un raccourci libre · votre choix est gardé pour la prochaine fois",
		"omegas Vorschlag steht oben · die anderen bekommen ein freies Kürzel · deine Wahl wird für das nächste Mal gespeichert",
		"A sugestão do omega vem primeiro · os outros passam para um atalho livre · a tua escolha fica guardada para a próxima vez",
	},

	"mac.suggested":    {"suggested", "sugerido", "suggéré", "empfohlen", "sugerido"},
	"mac.assumed":      {"the app's default shortcut; you may have changed it", "el atajo por defecto de la app; puede que lo hayas cambiado", "le raccourci par défaut de l'app ; vous l'avez peut-être changé", "das Standardkürzel der App; vielleicht hast du es geändert", "o atalho por omissão da app; talvez o tenhas mudado"},
	"mac.nobody":       {"Nobody: turn it off for all of them", "Nadie: desactivarlo en todas", "Personne : le désactiver partout", "Niemand: bei allen ausschalten", "Ninguém: desativá-lo em todas"},
	"mac.by_hand":      {"by hand", "a mano", "à la main", "von Hand", "à mão"},
	"mac.conflict":     {"%s is wanted by:", "%s lo quieren:", "%s est demandé par :", "%s wollen:", "%s é pedido por:"},
	"mac.no_conflicts": {"No two apps want the same shortcut.", "Ningún atajo lo quieren dos apps a la vez.", "Aucun raccourci n'est demandé par deux apps.", "Kein Kürzel wird von zwei Apps beansprucht.", "Nenhum atalho é pedido por duas apps."},
	"mac.manager":      {"You use %s: hide menu bar icons there.", "Usas %s: oculta ahí los iconos de la barra de menús.", "Vous utilisez %s : masquez-y les icônes de la barre des menus.", "Du nutzt %s: Blende Menüleisten-Symbole dort aus.", "Usas o %s: oculta lá os ícones da barra de menus."},
	"mac.dock_settings": {
		"autohide: %v · recent apps: %v · icon size: %d",
		"ocultar automáticamente: %v · apps recientes: %v · tamaño de iconos: %d",
		"masquage auto : %v · apps récentes : %v · taille des icônes : %d",
		"automatisch ausblenden: %v · zuletzt benutzte Apps: %v · Symbolgröße: %d",
		"ocultar automaticamente: %v · apps recentes: %v · tamanho dos ícones: %d",
	},
	"mac.not_installed": {"no longer installed", "ya no está instalada", "désinstallée", "nicht mehr installiert", "já não está instalada"},
	"mac.unused":        {"not opened in %d days", "sin abrir desde hace %d días", "pas ouverte depuis %d jours", "seit %d Tagen nicht geöffnet", "sem abrir há %d dias"},
	"mac.rules":         {"your rules", "tus reglas", "vos règles", "deine Regeln", "as tuas regras"},
	"mac.none_found":    {"Nothing to review here.", "Aquí no hay nada que revisar.", "Rien à passer en revue ici.", "Hier gibt es nichts zu prüfen.", "Aqui não há nada para rever."},
	"mac.nothing":       {"Nothing changed.", "No se ha cambiado nada.", "Rien n'a changé.", "Nichts geändert.", "Nada foi alterado."},
	"mac.failed":        {"⎿ failed (exit %d)", "⎿ falló (salida %d)", "⎿ échec (code %d)", "⎿ fehlgeschlagen (Exit %d)", "⎿ falhou (saída %d)"},
	"mac.undo_hint": {
		"Changes recorded: omega mac undo reverts them.",
		"Cambios registrados: omega mac undo los deshace.",
		"Changements enregistrés : omega mac undo les annule.",
		"Änderungen gespeichert: omega mac undo macht sie rückgängig.",
		"Alterações registadas: omega mac undo desfá-las.",
	},
	"mac.undo_empty": {"No changes to undo.", "No hay cambios que deshacer.", "Aucun changement à annuler.", "Keine Änderungen zum Rückgängigmachen.", "Não há alterações para desfazer."},
	"mac.undoing":    {"Revert: %s", "Deshacer: %s", "Annuler : %s", "Rückgängig: %s", "Desfazer: %s"},
	"mac.saved_rule": {
		"Your choices are saved in %s: omega will suggest the same next time.",
		"Tus elecciones se guardan en %s: omega sugerirá lo mismo la próxima vez.",
		"Vos choix sont enregistrés dans %s : omega suggérera la même chose la prochaine fois.",
		"Deine Wahl ist in %s gespeichert: omega schlägt beim nächsten Mal dasselbe vor.",
		"As tuas escolhas ficam guardadas em %s: o omega vai sugerir o mesmo da próxima vez.",
	},
	"mac.status_next": {
		"Change things with omega mac login|background|menubar|dock|shortcuts, or let the AI guide you with omega mac.",
		"Cambia cosas con omega mac login|background|menubar|dock|shortcuts, o deja que te guíe la IA con omega mac.",
		"Changez les choses avec omega mac login|background|menubar|dock|shortcuts, ou laissez l'IA vous guider avec omega mac.",
		"Ändere etwas mit omega mac login|background|menubar|dock|shortcuts oder lass dich mit omega mac von der KI führen.",
		"Muda as coisas com omega mac login|background|menubar|dock|shortcuts, ou deixa a IA guiar-te com omega mac.",
	},
	"mac.policy_defaults": {
		`omega's defaults (your rules in mac.toml win):
  · updaters (Google, Adobe, Microsoft, Zoom…) should not run at login or in the background
  · chat, media, browsers and AI assistants open when you need them, not at login
  · VPN, security, password managers, sync, input devices, launchers and menu bar managers stay
  · Apple's own items are never touched
  · Dock: apps no longer installed are removed; apps not opened in 90 days are suggested
  · shortcuts: essential macOS shortcuts first, then the ones you set on purpose, then launchers and
    utilities; apps that claim a shortcut when installed (AI assistants) move to a free one`,
		`Valores por defecto de omega (tus reglas en mac.toml mandan):
  · los actualizadores (Google, Adobe, Microsoft, Zoom…) no deben ejecutarse al inicio ni en segundo plano
  · chat, multimedia, navegadores y asistentes de IA se abren cuando los necesitas, no al iniciar sesión
  · VPN, seguridad, gestores de contraseñas, sincronización, periféricos, lanzadores y gestores de la barra de menús se quedan
  · nunca se toca lo de Apple
  · Dock: se quitan las apps que ya no están instaladas; se sugieren las que no abres desde hace 90 días
  · atajos: primero los esenciales de macOS, después los que configuraste a propósito, después lanzadores y
    utilidades; las apps que se adueñan de un atajo al instalarse (asistentes de IA) pasan a uno libre`,
		`Valeurs par défaut d'omega (vos règles dans mac.toml priment) :
  · les programmes de mise à jour (Google, Adobe, Microsoft, Zoom…) ne doivent pas tourner à la connexion ni en arrière-plan
  · chat, médias, navigateurs et assistants IA s'ouvrent quand vous en avez besoin, pas à la connexion
  · VPN, sécurité, gestionnaires de mots de passe, synchronisation, périphériques, lanceurs et gestionnaires de barre des menus restent
  · rien d'Apple n'est touché
  · Dock : les apps désinstallées sont retirées ; celles non ouvertes depuis 90 jours sont suggérées
  · raccourcis : d'abord les essentiels de macOS, puis ceux que vous avez choisis, puis lanceurs et
    utilitaires ; les apps qui s'emparent d'un raccourci à l'installation (assistants IA) passent à un raccourci libre`,
		`omegas Standardwerte (deine Regeln in mac.toml haben Vorrang):
  · Updater (Google, Adobe, Microsoft, Zoom…) sollen weder beim Anmelden noch im Hintergrund laufen
  · Chat, Medien, Browser und KI-Assistenten öffnest du bei Bedarf, nicht beim Anmelden
  · VPN, Sicherheit, Passwortmanager, Synchronisierung, Eingabegeräte, Launcher und Menüleisten-Manager bleiben
  · Apples eigene Objekte werden nie angefasst
  · Dock: nicht mehr installierte Apps werden entfernt; seit 90 Tagen ungeöffnete Apps werden vorgeschlagen
  · Kürzel: zuerst die wichtigen von macOS, dann die bewusst gesetzten, dann Launcher und
    Werkzeuge; Apps, die sich bei der Installation ein Kürzel nehmen (KI-Assistenten), bekommen ein freies`,
		`Valores por omissão do omega (as tuas regras em mac.toml prevalecem):
  · os atualizadores (Google, Adobe, Microsoft, Zoom…) não devem correr no início nem em segundo plano
  · chat, multimédia, navegadores e assistentes de IA abrem quando precisas, não no início de sessão
  · VPN, segurança, gestores de palavras-passe, sincronização, periféricos, lançadores e gestores da barra de menus ficam
  · nada da Apple é tocado
  · Dock: as apps desinstaladas saem; sugerem-se as que não abres há 90 dias
  · atalhos: primeiro os essenciais do macOS, depois os que definiste de propósito, depois lançadores e
    utilitários; as apps que se apropriam de um atalho ao instalar (assistentes de IA) passam para um livre`,
	},
	"mac.policy_path": {"Your rules (%s):", "Tus reglas (%s):", "Vos règles (%s) :", "Deine Regeln (%s):", "As tuas regras (%s):"},
	"mac.policy_none": {
		"You have no rules of your own yet. Create them with omega mac policy edit, or let omega mac save your choices.",
		"Aún no tienes reglas propias. Créalas con omega mac policy edit, o deja que omega mac guarde tus elecciones.",
		"Vous n'avez pas encore de règles. Créez-les avec omega mac policy edit, ou laissez omega mac garder vos choix.",
		"Du hast noch keine eigenen Regeln. Lege sie mit omega mac policy edit an oder lass omega mac deine Wahl speichern.",
		"Ainda não tens regras próprias. Cria-as com omega mac policy edit, ou deixa o omega mac guardar as tuas escolhas.",
	},
}

func init() {
	for k, v := range macMessages {
		messages[k] = v
	}
}
