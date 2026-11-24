package i18n

// Messages of the command line itself: the help templates and the errors of
// a command line that does not parse, the tasks as commands, the runs
// without the AI, history, logs, notes compact, steps show, named routines
// and the names of the steps that had none. Same order as messages: en, es,
// fr, de, pt. A key ending in ".one" is the singular of the key before it
// (see N).
var cliMessages = map[string][]string{
	"help.task_word": {"task", "tarea", "tâche", "Aufgabe", "tarefa"},
	// ---- help templates ----
	"help.usage":         {"Usage", "Uso", "Utilisation", "Verwendung", "Utilização"},
	"help.command_arg":   {"[command]", "[orden]", "[commande]", "[Befehl]", "[comando]"},
	"help.flags_arg":     {"[options]", "[opciones]", "[options]", "[Optionen]", "[opções]"},
	"help.aliases":       {"Aliases", "Alias", "Alias", "Aliasse", "Alias"},
	"help.examples":      {"Examples", "Ejemplos", "Exemples", "Beispiele", "Exemplos"},
	"help.commands":      {"Commands", "Órdenes", "Commandes", "Befehle", "Comandos"},
	"help.more_commands": {"Other commands", "Otras órdenes", "Autres commandes", "Weitere Befehle", "Outros comandos"},
	"help.flags":         {"Options", "Opciones", "Options", "Optionen", "Opções"},
	"help.flags_agent":   {"Agent options", "Opciones del agente", "Options de l'agent", "Agent-Optionen", "Opções do agente"},
	"help.flags_noai":    {"Options without AI (with --no-ai)", "Opciones sin IA (con --no-ai)", "Options sans IA (avec --no-ai)", "Optionen ohne KI (mit --no-ai)", "Opções sem IA (com --no-ai)"},
	"help.flags_global":  {"Global options", "Opciones globales", "Options globales", "Globale Optionen", "Opções globais"},
	"help.see_also":      {"See also", "Ver también", "Voir aussi", "Siehe auch", "Ver também"},
	"help.more": {
		"Use \"%s\" for more about a command.", "Usa «%s» para saber más de una orden.", "Utilisez « %s » pour en savoir plus sur une commande.",
		"Mehr zu einem Befehl: „%s“.", "Usa «%s» para saber mais sobre um comando.",
	},
	"cmd.help":     {"Help about any command", "Ayuda de cualquier orden", "Aide sur une commande", "Hilfe zu einem Befehl", "Ajuda sobre qualquer comando"},
	"flag.help":    {"help for %s", "ayuda de %s", "aide de %s", "Hilfe zu %s", "ajuda de %s"},
	"flag.version": {"show omega's version", "mostrar la versión de omega", "afficher la version d'omega", "die Version von omega zeigen", "mostrar a versão do omega"},
	"cli.exit_codes": {
		"Exit status: 0 the run worked · 1 something failed · 2 warnings or outdated packages\n(--fail-on-warning, --fail-on-outdated) · 64 a command line omega cannot use.",
		"Código de salida: 0 todo bien · 1 algo falló · 2 avisos o paquetes desactualizados\n(--fail-on-warning, --fail-on-outdated) · 64 una línea de órdenes que omega no entiende.",
		"Code de sortie : 0 tout va bien · 1 un échec · 2 avertissements ou paquets obsolètes\n(--fail-on-warning, --fail-on-outdated) · 64 une ligne de commande qu'omega ne comprend pas.",
		"Exit-Status: 0 alles gut · 1 etwas ist fehlgeschlagen · 2 Warnungen oder veraltete Pakete\n(--fail-on-warning, --fail-on-outdated) · 64 eine Befehlszeile, die omega nicht versteht.",
		"Código de saída: 0 tudo bem · 1 algo falhou · 2 avisos ou pacotes desatualizados\n(--fail-on-warning, --fail-on-outdated) · 64 uma linha de comandos que o omega não entende.",
	},

	// ---- errors of the command line ----
	"err.see_help":    {"See: %s", "Mira: %s", "Voir : %s", "Siehe: %s", "Vê: %s"},
	"err.unknown_sub": {"unknown command %q for %q.", "orden desconocida %q para %q.", "commande inconnue %q pour %q.", "unbekannter Befehl %q für %q.", "comando desconhecido %q para %q."},
	"err.unknown_word": {
		"omega has no command or task %q.", "omega no tiene ninguna orden ni tarea %q.", "omega n'a ni commande ni tâche %q.",
		"omega hat weder Befehl noch Aufgabe %q.", "o omega não tem nenhum comando nem tarefa %q.",
	},
	"err.flag_unknown":     {"unknown option %s.", "opción desconocida %s.", "option inconnue %s.", "unbekannte Option %s.", "opção desconhecida %s."},
	"err.flag_needs_value": {"option %s needs a value.", "la opción %s necesita un valor.", "l'option %s demande une valeur.", "Option %s braucht einen Wert.", "a opção %s precisa de um valor."},
	"err.flag_bad_value": {
		"%q is not a valid value for %s.", "%q no es un valor válido para %s.", "%q n'est pas une valeur valide pour %s.",
		"%q ist kein gültiger Wert für %s.", "%q não é um valor válido para %s.",
	},
	"err.flag_not_here": {
		"%s belongs to the runs without AI, and %s does not have it.", "%s es de las ejecuciones sin IA, y %s no la tiene.",
		"%s sert aux exécutions sans IA, et %s ne l'a pas.", "%s gehört zu den Läufen ohne KI, und %s hat sie nicht.",
		"%s é das execuções sem IA, e %s não a tem.",
	},
	"err.flags_together": {
		"these options cannot be used together: %s.", "estas opciones no se pueden usar juntas: %s.", "ces options ne peuvent pas être utilisées ensemble : %s.",
		"diese Optionen gehen nicht zusammen: %s.", "estas opções não podem ser usadas juntas: %s.",
	},
	"err.args_none": {
		"%s takes no arguments, and got %q.", "%s no lleva argumentos, y recibió %q.", "%s ne prend pas d'argument, et a reçu %q.",
		"%s nimmt keine Argumente, bekam aber %q.", "%s não leva argumentos, e recebeu %q.",
	},
	"err.args_at_most": {
		"%s takes at most %d arguments; it got %d.", "%s lleva como mucho %d argumentos; recibió %d.", "%s prend au plus %d arguments ; il en a reçu %d.",
		"%s nimmt höchstens %d Argumente; es bekam %d.", "%s leva no máximo %d argumentos; recebeu %d.",
	},
	"err.args_at_most.one": {
		"%s takes at most %d argument; it got %d.", "%s lleva como mucho %d argumento; recibió %d.", "%s prend au plus %d argument ; il en a reçu %d.",
		"%s nimmt höchstens %d Argument; es bekam %d.", "%s leva no máximo %d argumento; recebeu %d.",
	},
	"err.args_exact": {
		"%s takes %d arguments; it got %d.", "%s lleva %d argumentos; recibió %d.", "%s prend %d arguments ; il en a reçu %d.",
		"%s nimmt %d Argumente; es bekam %d.", "%s leva %d argumentos; recebeu %d.",
	},
	"err.args_exact.one": {
		"%s takes %d argument; it got %d.", "%s lleva %d argumento; recibió %d.", "%s prend %d argument ; il en a reçu %d.",
		"%s nimmt %d Argument; es bekam %d.", "%s leva %d argumento; recebeu %d.",
	},
	"err.args_at_least": {
		"%s needs at least %d arguments; it got %d.", "%s necesita al menos %d argumentos; recibió %d.", "%s demande au moins %d arguments ; il en a reçu %d.",
		"%s braucht mindestens %d Argumente; es bekam %d.", "%s precisa de pelo menos %d argumentos; recebeu %d.",
	},
	"err.args_at_least.one": {
		"%s needs at least %d argument; it got %d.", "%s necesita al menos %d argumento; recibió %d.", "%s demande au moins %d argument ; il en a reçu %d.",
		"%s braucht mindestens %d Argument; es bekam %d.", "%s precisa de pelo menos %d argumento; recebeu %d.",
	},
	"err.args_between": {
		"%s takes between %d and %d arguments; it got %d.", "%s lleva entre %d y %d argumentos; recibió %d.", "%s prend entre %d et %d arguments ; il en a reçu %d.",
		"%s nimmt %d bis %d Argumente; es bekam %d.", "%s leva entre %d e %d argumentos; recebeu %d.",
	},
	"err.args_invalid": {
		"%q is not something %s takes.", "%q no es algo que %s acepte.", "%q n'est pas accepté par %s.",
		"%q nimmt %s nicht an.", "%q não é algo que %s aceite.",
	},
	"warn.noai_flags": {
		"%s only change a run without AI (--no-ai); this run uses the AI and ignores them.",
		"%s solo cambian una ejecución sin IA (--no-ai); esta usa la IA y no las tiene en cuenta.",
		"%s ne changent qu'une exécution sans IA (--no-ai) ; celle-ci utilise l'IA et les ignore.",
		"%s wirken nur in einem Lauf ohne KI (--no-ai); dieser nutzt die KI und ignoriert sie.",
		"%s só mudam uma execução sem IA (--no-ai); esta usa a IA e ignora-as.",
	},

	// ---- run flags ----
	"flag.max_cost": {
		"stop at this estimated cost in USD (0: no limit)", "parar al llegar a este coste estimado en USD (0: sin límite)",
		"s'arrêter à ce coût estimé en USD (0 : sans limite)", "bei diesen geschätzten Kosten in USD aufhören (0: ohne Limit)",
		"parar ao chegar a este custo estimado em USD (0: sem limite)",
	},
	"flag.max_turns": {
		"stop after this many turns of the model (0: no limit)", "parar tras este número de turnos del modelo (0: sin límite)",
		"s'arrêter après ce nombre de tours du modèle (0 : sans limite)", "nach so vielen Runden des Modells aufhören (0: ohne Limit)",
		"parar após este número de turnos do modelo (0: sem limite)",
	},
	"flag.no_ai": {
		"run the built-in steps without calling any AI model", "ejecutar los pasos integrados sin llamar a ningún modelo de IA",
		"exécuter les étapes intégrées sans appeler de modèle d'IA", "die eingebauten Schritte ohne KI-Modell ausführen",
		"executar os passos integrados sem chamar nenhum modelo de IA",
	},
	"flag.json_run": {
		"print the report as JSON (runs without AI, like --no-ai)", "escribir el informe en JSON (sin IA, como --no-ai)",
		"écrire le rapport en JSON (sans IA, comme --no-ai)", "den Bericht als JSON ausgeben (ohne KI, wie --no-ai)",
		"escrever o relatório em JSON (sem IA, como --no-ai)",
	},
	"flag.format": {
		"report format: text (the default) or json (as --json)", "formato del informe: text (por defecto) o json (como --json)",
		"format du rapport : text (par défaut) ou json (comme --json)", "Berichtsformat: text (Standard) oder json (wie --json)",
		"formato do relatório: text (por omissão) ou json (como --json)",
	},
	"flag.quiet": {
		"print only what matters: no progress, no notes about hooks", "escribir solo lo importante: sin progreso ni avisos de hooks",
		"n'écrire que l'essentiel : ni progression, ni notes sur les hooks", "nur das Wichtige ausgeben: kein Fortschritt, keine Hook-Hinweise",
		"escrever só o essencial: sem progresso nem avisos de hooks",
	},
	"flag.verbose": {
		"show the output of every command", "mostrar la salida de cada comando", "afficher la sortie de chaque commande",
		"die Ausgabe jedes Befehls zeigen", "mostrar a saída de cada comando",
	},
	"flag.fail_warning": {
		"exit with 2 when a check warns", "salir con 2 si una comprobación avisa", "sortir avec 2 si une vérification avertit",
		"mit 2 beenden, wenn eine Prüfung warnt", "sair com 2 se uma verificação avisar",
	},
	"flag.fail_outdated": {
		"exit with 2 when packages are outdated", "salir con 2 si hay paquetes desactualizados", "sortir avec 2 si des paquets sont obsolètes",
		"mit 2 beenden, wenn Pakete veraltet sind", "sair com 2 se houver pacotes desatualizados",
	},
	"flag.fail_critical": {
		"exit with 1 when any step fails, checks included", "salir con 1 si falla cualquier paso, comprobaciones incluidas",
		"sortir avec 1 si une étape échoue, vérifications comprises", "mit 1 beenden, wenn ein Schritt fehlschlägt, Prüfungen eingeschlossen",
		"sair com 1 se falhar qualquer passo, verificações incluídas",
	},
	"flag.json": {"print the answer as JSON", "escribir la respuesta en JSON", "écrire la réponse en JSON", "die Antwort als JSON ausgeben", "escrever a resposta em JSON"},
	"det.no_key": {
		"running without AI (the built-in steps only)", "se ejecuta sin IA (solo los pasos integrados)", "exécution sans IA (les étapes intégrées seulement)",
		"läuft ohne KI (nur die eingebauten Schritte)", "a executar sem IA (só os passos integrados)",
	},

	// ---- the tasks as commands ----
	"cmd.task.update": {"Update your software, nothing else", "Actualizar tu software, nada más", "Mettre à jour vos logiciels, rien d'autre", "Deine Software aktualisieren, sonst nichts", "Atualizar o teu software, mais nada"},
	"cmd.task.update_long": {
		"Updates the package managers and the software they installed (Homebrew, npm, pipx, the system…), like topgrade.\nWith --no-ai it runs the built-in update steps; with an AI model, the model runs them and deals with what fails.",
		"Actualiza los gestores de paquetes y el software que instalaron (Homebrew, npm, pipx, el sistema…), como topgrade.\nCon --no-ai ejecuta los pasos de actualización integrados; con un modelo de IA, el modelo los ejecuta y se ocupa de lo que falle.",
		"Met à jour les gestionnaires de paquets et ce qu'ils ont installé (Homebrew, npm, pipx, le système…), comme topgrade.\nAvec --no-ai, exécute les étapes de mise à jour intégrées ; avec un modèle d'IA, le modèle les exécute et traite les échecs.",
		"Aktualisiert die Paketmanager und was sie installiert haben (Homebrew, npm, pipx, das System…), wie topgrade.\nMit --no-ai laufen die eingebauten Update-Schritte; mit einem KI-Modell führt das Modell sie aus und kümmert sich um Fehler.",
		"Atualiza os gestores de pacotes e o software que instalaram (Homebrew, npm, pipx, o sistema…), como o topgrade.\nCom --no-ai executa os passos de atualização integrados; com um modelo de IA, o modelo executa-os e trata do que falhar.",
	},
	"cmd.task.clean": {"Clean caches and leftovers, nothing else", "Limpiar cachés y restos, nada más", "Nettoyer caches et restes, rien d'autre", "Caches und Reste aufräumen, sonst nichts", "Limpar caches e restos, mais nada"},
	"cmd.task.clean_long": {
		"Cleans what is left over: caches, old versions, build folders, the Trash (asking first), like mo clean.\nWith --no-ai it runs the built-in cleanup steps; the ones that delete for good always ask.",
		"Limpia lo que sobra: cachés, versiones antiguas, carpetas de compilación, la Papelera (preguntando antes), como mo clean.\nCon --no-ai ejecuta los pasos de limpieza integrados; los que borran para siempre preguntan siempre.",
		"Nettoie ce qui reste : caches, anciennes versions, dossiers de build, la Corbeille (en demandant d'abord), comme mo clean.\nAvec --no-ai, exécute les étapes de nettoyage intégrées ; celles qui suppriment définitivement demandent toujours.",
		"Räumt Übriggebliebenes auf: Caches, alte Versionen, Build-Ordner, den Papierkorb (mit Rückfrage), wie mo clean.\nMit --no-ai laufen die eingebauten Aufräumschritte; was endgültig löscht, fragt immer.",
		"Limpa o que sobra: caches, versões antigas, pastas de build, o Lixo (perguntando antes), como o mo clean.\nCom --no-ai executa os passos de limpeza integrados; os que apagam para sempre perguntam sempre.",
	},
	"cmd.task.analyze": {"Report on the machine without changing anything", "Informe del equipo sin cambiar nada", "Rapport sur la machine sans rien changer", "Bericht über den Rechner, ohne etwas zu ändern", "Relatório da máquina sem mudar nada"},
	"cmd.task.analyze_long": {
		"Looks at the machine and reports: disk, what is outdated, what failed lately. It runs only commands that read.\nWith --no-ai it runs the built-in checks and prints their output; --json gives the report as data.",
		"Revisa el equipo e informa: disco, lo desactualizado, lo que falló hace poco. Solo ejecuta órdenes que leen.\nCon --no-ai ejecuta las comprobaciones integradas y muestra su salida; --json da el informe como datos.",
		"Examine la machine et fait un rapport : disque, ce qui est obsolète, ce qui a échoué récemment. N'exécute que des commandes qui lisent.\nAvec --no-ai, exécute les vérifications intégrées et affiche leur sortie ; --json donne le rapport en données.",
		"Sieht sich den Rechner an und berichtet: Platte, Veraltetes, was zuletzt fehlschlug. Es laufen nur lesende Befehle.\nMit --no-ai laufen die eingebauten Prüfungen mit ihrer Ausgabe; --json liefert den Bericht als Daten.",
		"Revê a máquina e informa: disco, o que está desatualizado, o que falhou há pouco. Só executa comandos que leem.\nCom --no-ai executa as verificações integradas e mostra a saída; --json dá o relatório como dados.",
	},
	"cmd.task.health": {"Check-up of the hardware and the security", "Revisión del hardware y la seguridad", "Bilan du matériel et de la sécurité", "Check-up von Hardware und Sicherheit", "Revisão do hardware e da segurança"},
	"cmd.task.health_long": {
		"Checks the battery, the disk, the memory and the security settings (firewall, FileVault, updates).\nWith --no-ai it runs the built-in health checks; with an AI model, the model explains what it finds.",
		"Revisa la batería, el disco, la memoria y los ajustes de seguridad (cortafuegos, FileVault, actualizaciones).\nCon --no-ai ejecuta las comprobaciones de salud integradas; con un modelo de IA, el modelo explica lo que encuentra.",
		"Vérifie la batterie, le disque, la mémoire et les réglages de sécurité (pare-feu, FileVault, mises à jour).\nAvec --no-ai, exécute les vérifications intégrées ; avec un modèle d'IA, le modèle explique ce qu'il trouve.",
		"Prüft Akku, Platte, Speicher und die Sicherheitseinstellungen (Firewall, FileVault, Updates).\nMit --no-ai laufen die eingebauten Prüfungen; mit einem KI-Modell erklärt das Modell, was es findet.",
		"Verifica a bateria, o disco, a memória e as definições de segurança (firewall, FileVault, atualizações).\nCom --no-ai executa as verificações integradas; com um modelo de IA, o modelo explica o que encontra.",
	},
	"cmd.task.tune": {"Review your tools' configuration (needs AI)", "Revisar la configuración de tus herramientas (con IA)", "Revoir la configuration de vos outils (avec IA)", "Konfiguration deiner Tools prüfen (mit KI)", "Rever a configuração das ferramentas (com IA)"},
	"cmd.task.tune_long": {
		"The model reviews the configuration of your tools (topgrade, Mole, your shell, Git…) and proposes improvements;\nnothing changes without your yes. It needs an AI model: there is no --no-ai version.",
		"El modelo revisa la configuración de tus herramientas (topgrade, Mole, tu shell, Git…) y propone mejoras;\nnada cambia sin tu sí. Necesita un modelo de IA: no hay versión --no-ai.",
		"Le modèle examine la configuration de vos outils (topgrade, Mole, votre shell, Git…) et propose des améliorations ;\nrien ne change sans votre accord. Il faut un modèle d'IA : pas de version --no-ai.",
		"Das Modell prüft die Konfiguration deiner Tools (topgrade, Mole, deine Shell, Git…) und schlägt Verbesserungen vor;\nnichts ändert sich ohne dein Ja. Braucht ein KI-Modell: keine --no-ai-Variante.",
		"O modelo revê a configuração das tuas ferramentas (topgrade, Mole, a tua shell, Git…) e propõe melhorias;\nnada muda sem o teu sim. Precisa de um modelo de IA: não há versão --no-ai.",
	},
	"cmd.fix_long": {
		"The model diagnoses an error and fixes it, asking before it changes anything:\n  omega fix \"…\"          the error, in your words or pasted\n  some-command 2>&1 | omega fix -   the error read from stdin\n  omega fix --log FILE   a build or command log\n  omega fix              with nothing given: why topgrade fails, from its dry run and configuration\nfix needs an AI model; the options of the runs without AI do not apply.",
		"El modelo diagnostica un error y lo arregla, preguntando antes de cambiar nada:\n  omega fix \"…\"          el error, con tus palabras o pegado\n  orden 2>&1 | omega fix -   el error leído de la entrada estándar\n  omega fix --log FICHERO   un registro de compilación o de una orden\n  omega fix              sin nada más: por qué falla topgrade, según su simulación y su configuración\nfix necesita un modelo de IA; las opciones de las ejecuciones sin IA no se aplican.",
		"Le modèle diagnostique une erreur et la corrige, en demandant avant de changer quoi que ce soit :\n  omega fix \"…\"          l'erreur, avec vos mots ou collée\n  commande 2>&1 | omega fix -   l'erreur lue sur l'entrée standard\n  omega fix --log FICHIER   un journal de build ou de commande\n  omega fix              sans rien : pourquoi topgrade échoue, d'après sa simulation et sa configuration\nfix demande un modèle d'IA ; les options des exécutions sans IA ne s'appliquent pas.",
		"Das Modell diagnostiziert einen Fehler und behebt ihn, mit Rückfrage vor jeder Änderung:\n  omega fix \"…\"          der Fehler, in deinen Worten oder eingefügt\n  befehl 2>&1 | omega fix -   der Fehler von der Standardeingabe\n  omega fix --log DATEI  ein Build- oder Befehlsprotokoll\n  omega fix              ohne Angabe: warum topgrade fehlschlägt, aus Probelauf und Konfiguration\nfix braucht ein KI-Modell; die Optionen der Läufe ohne KI gelten nicht.",
		"O modelo diagnostica um erro e corrige-o, perguntando antes de mudar algo:\n  omega fix \"…\"          o erro, pelas tuas palavras ou colado\n  comando 2>&1 | omega fix -   o erro lido da entrada padrão\n  omega fix --log FICHEIRO   um registo de build ou de um comando\n  omega fix              sem mais nada: porque falha o topgrade, pela simulação e configuração\nfix precisa de um modelo de IA; as opções das execuções sem IA não se aplicam.",
	},
	"flag.fix_log": {"a log file with the error to diagnose", "un fichero de registro con el error a diagnosticar", "un fichier journal avec l'erreur à diagnostiquer", "eine Protokolldatei mit dem Fehler", "um ficheiro de registo com o erro a diagnosticar"},

	// ---- runs without the AI ----
	"det.no_ai":     {"without AI", "sin IA", "sans IA", "ohne KI", "sem IA"},
	"det.h_update":  {"Update", "Actualización", "Mise à jour", "Aktualisierung", "Atualização"},
	"det.h_clean":   {"Cleanup", "Limpieza", "Nettoyage", "Aufräumen", "Limpeza"},
	"det.h_analyze": {"Analysis", "Análisis", "Analyse", "Analyse", "Análise"},
	"det.h_health":  {"Health", "Salud del equipo", "Santé", "Zustand", "Saúde"},
	"det.h_quick":   {"Quick safe maintenance", "Mantenimiento rápido y seguro", "Maintenance rapide et sûre", "Schnelle sichere Wartung", "Manutenção rápida e segura"},
	"det.skipped": {
		"%s (skipped: needs a confirmation at a terminal)", "%s (omitido: necesita una confirmación en un terminal)", "%s (ignoré : demande une confirmation dans un terminal)",
		"%s (übersprungen: braucht eine Bestätigung am Terminal)", "%s (ignorado: precisa de uma confirmação num terminal)",
	},
	"det.reason_tty":      {"needs a confirmation at a terminal", "necesita una confirmación en un terminal", "demande une confirmation dans un terminal", "braucht eine Bestätigung am Terminal", "precisa de uma confirmação num terminal"},
	"det.reason_declined": {"declined", "rechazado", "refusé", "abgelehnt", "recusado"},
	"det.would_run":       {"[dry-run] would run: %s (%s)", "[dry-run] se ejecutaría: %s (%s)", "[dry-run] exécuterait : %s (%s)", "[dry-run] würde ausführen: %s (%s)", "[dry-run] executaria: %s (%s)"},
	"det.battery":         {"Battery", "Batería", "Batterie", "Akku", "Bateria"},
	"det.battery_health":  {"Battery health", "Salud de la batería", "Santé de la batterie", "Akkuzustand", "Saúde da bateria"},
	"det.disk":            {"Disk", "Disco", "Disque", "Platte", "Disco"},
	"det.check_failed":    {"%s: could not be checked (exit %d)", "%s: no se pudo comprobar (exit %d)", "%s : impossible à vérifier (exit %d)", "%s: konnte nicht geprüft werden (exit %d)", "%s: não foi possível verificar (exit %d)"},
	"det.quick_intro": {
		"Running the safe cleanups, then checking the battery, the disk and pending updates…",
		"Primero las limpiezas seguras; después, la batería, el disco y las actualizaciones pendientes…",
		"D'abord les nettoyages sûrs, puis la batterie, le disque et les mises à jour en attente…",
		"Zuerst die sicheren Aufräumschritte, dann Akku, Platte und anstehende Updates…",
		"Primeiro as limpezas seguras; depois, a bateria, o disco e as atualizações pendentes…",
	},
	"det.w_battery":   {"battery", "batería", "batterie", "Akku", "bateria"},
	"det.w_disk":      {"disk", "disco", "disque", "Platte", "disco"},
	"det.w_updates":   {"pending updates", "actualizaciones pendientes", "mises à jour en attente", "anstehende Updates", "atualizações pendentes"},
	"det.w_nothing":   {"nothing", "nada", "rien", "nichts", "nada"},
	"det.no_problems": {"No problems found.", "No se encontraron problemas.", "Aucun problème trouvé.", "Keine Probleme gefunden.", "Não foram encontrados problemas."},
	"det.cleanups_failed": {
		"%d cleanup steps failed", "fallaron %d pasos de limpieza", "%d étapes de nettoyage ont échoué", "%d Aufräumschritte fehlgeschlagen", "falharam %d passos de limpeza",
	},
	"det.cleanups_failed.one": {
		"%d cleanup step failed", "falló %d paso de limpieza", "%d étape de nettoyage a échoué", "%d Aufräumschritt fehlgeschlagen", "falhou %d passo de limpeza",
	},
	"det.checks_failed": {
		"%d checks could not be run", "%d comprobaciones no se pudieron hacer", "%d vérifications n'ont pas pu être faites", "%d Prüfungen konnten nicht laufen", "%d verificações não puderam ser feitas",
	},
	"det.checks_failed.one": {
		"%d check could not be run", "%d comprobación no se pudo hacer", "%d vérification n'a pas pu être faite", "%d Prüfung konnte nicht laufen", "%d verificação não pôde ser feita",
	},
	"det.sum_quick": {
		"Cleanups run: %d · checked: %s · updates pending: %d.", "Limpiezas hechas: %d · revisado: %s · actualizaciones pendientes: %d.",
		"Nettoyages faits : %d · vérifié : %s · mises à jour en attente : %d.", "Aufräumschritte: %d · geprüft: %s · anstehende Updates: %d.",
		"Limpezas feitas: %d · verificado: %s · atualizações pendentes: %d.",
	},
	"det.quick_skipped": {
		"%d cleanups were skipped: they need a yes at a terminal, or --yes.", "Se omitieron %d limpiezas: necesitan un sí en un terminal, o --yes.",
		"%d nettoyages ignorés : ils demandent un oui dans un terminal, ou --yes.", "%d Aufräumschritte übersprungen: sie brauchen ein Ja am Terminal oder --yes.",
		"Foram ignoradas %d limpezas: precisam de um sim num terminal, ou --yes.",
	},
	"det.quick_skipped.one": {
		"%d cleanup was skipped: it needs a yes at a terminal, or --yes.", "Se omitió %d limpieza: necesita un sí en un terminal, o --yes.",
		"%d nettoyage ignoré : il demande un oui dans un terminal, ou --yes.", "%d Aufräumschritt übersprungen: er braucht ein Ja am Terminal oder --yes.",
		"Foi ignorada %d limpeza: precisa de um sim num terminal, ou --yes.",
	},
	"det.hint_update": {
		"Run omega update to update your software.", "Ejecuta omega update para actualizar tu software.", "Lancez omega update pour mettre à jour vos logiciels.",
		"Starte omega update, um deine Software zu aktualisieren.", "Executa omega update para atualizar o teu software.",
	},
	"det.sum_full": {
		"Full maintenance: steps run: %d · failed: %d · outdated packages: %d", "Mantenimiento completo: pasos ejecutados: %d · fallidos: %d · paquetes desactualizados: %d",
		"Maintenance complète : étapes exécutées : %d · échecs : %d · paquets obsolètes : %d", "Komplette Wartung: Schritte: %d · fehlgeschlagen: %d · veraltete Pakete: %d",
		"Manutenção completa: passos executados: %d · falhados: %d · pacotes desatualizados: %d",
	},
	"det.sum_update": {
		"Update: %d steps run", "Actualización: %d pasos ejecutados", "Mise à jour : %d étapes exécutées", "Aktualisierung: %d Schritte ausgeführt", "Atualização: %d passos executados",
	},
	"det.sum_update.one": {
		"Update: %d step run", "Actualización: %d paso ejecutado", "Mise à jour : %d étape exécutée", "Aktualisierung: %d Schritt ausgeführt", "Atualização: %d passo executado",
	},
	"det.sum_clean": {
		"Cleanup: %d steps run", "Limpieza: %d pasos ejecutados", "Nettoyage : %d étapes exécutées", "Aufräumen: %d Schritte ausgeführt", "Limpeza: %d passos executados",
	},
	"det.sum_clean.one": {
		"Cleanup: %d step run", "Limpieza: %d paso ejecutado", "Nettoyage : %d étape exécutée", "Aufräumen: %d Schritt ausgeführt", "Limpeza: %d passo executado",
	},
	"det.sum_analyze": {
		"Analysis: checks run: %d · outdated packages: %d", "Análisis: comprobaciones hechas: %d · paquetes desactualizados: %d",
		"Analyse : vérifications faites : %d · paquets obsolètes : %d", "Analyse: Prüfungen: %d · veraltete Pakete: %d",
		"Análise: verificações feitas: %d · pacotes desatualizados: %d",
	},
	"det.sum_health": {
		"Health: checks run: %d · with warnings: %d", "Salud: comprobaciones hechas: %d · con avisos: %d", "Santé : vérifications faites : %d · avec avertissements : %d",
		"Zustand: Prüfungen: %d · mit Warnungen: %d", "Saúde: verificações feitas: %d · com avisos: %d",
	},
	"det.sum_custom": {
		"Routine %s: steps run: %d", "Rutina %s: pasos ejecutados: %d", "Routine %s : étapes exécutées : %d", "Routine %s: Schritte ausgeführt: %d", "Rotina %s: passos executados: %d",
	},

	// ---- plurals of older texts ----
	"steps.failed_summary.one": {
		"%d step failed; try `omega fix` or `omega -p \"...\"` to let the AI diagnose it",
		"%d paso falló; prueba `omega fix` o `omega -p \"...\"` para que la IA lo diagnostique",
		"%d étape en échec ; essayez `omega fix` ou `omega -p \"...\"` pour que l'IA diagnostique",
		"%d Schritt fehlgeschlagen; versuche `omega fix` oder `omega -p \"...\"`, damit die KI es diagnostiziert",
		"%d passo falhou; experimente `omega fix` ou `omega -p \"...\"` para a IA diagnosticar",
	},
	"steps.skipped_summary.one": {
		"%d step did not run: it needs a yes at a terminal", "%d paso no se ejecutó: necesita un sí en un terminal",
		"%d étape non exécutée : elle demande un oui dans un terminal", "%d Schritt nicht ausgeführt: er braucht ein Ja am Terminal",
		"%d passo não foi executado: precisa de um sim num terminal",
	},
	"pending.seen.one": {
		"since %s, seen in %d run", "desde el %s, visto en %d ejecución", "depuis le %s, vu dans %d exécution",
		"seit %s, in %d Lauf gesehen", "desde %s, visto em %d execução",
	},
	"logs.header.one": {
		"%d run log in %s (keeping the newest %d)", "%d registro en %s (se conservan los %d más recientes)", "%d journal dans %s (les %d plus récents sont conservés)",
		"%d Protokoll in %s (die %d neuesten bleiben erhalten)", "%d registo em %s (ficam os %d mais recentes)",
	},
	"logs.header2.one": {
		"%d run (the newest %d are kept)", "%d ejecución (se guardan las %d últimas)", "%d exécution (les %d plus récentes sont gardées)",
		"%d Lauf (die neuesten %d bleiben)", "%d execução (ficam as %d mais recentes)",
	},
	"doctor.failed.one": {"%d check failed", "%d comprobación fallida", "%d vérification en échec", "%d Prüfung fehlgeschlagen", "%d verificação falhada"},
	"docker.confirm.one": {
		"Run %d cleanup? This cannot be undone. y/N", "¿Ejecutar %d limpieza? No se puede deshacer. s/N", "Lancer %d nettoyage ? C'est irréversible. o/N",
		"%d Bereinigung ausführen? Das kann nicht rückgängig gemacht werden. j/N", "Executar %d limpeza? Não se pode desfazer. s/N",
	},
	"bp.failed.one": {"bootstrap: %d step failed", "bootstrap: %d paso falló", "bootstrap : %d étape en échec", "bootstrap: %d Schritt fehlgeschlagen", "bootstrap: %d passo falhou"},
	"custom.saved.one": {
		"Routine saved: %d step in %s", "Rutina guardada: %d paso en %s", "Routine enregistrée : %d étape dans %s",
		"Routine gespeichert: %d Schritt in %s", "Rotina guardada: %d passo em %s",
	},

	// ---- logs ----
	"cmd.logs_long": {
		"Lists the transcripts of earlier runs, newest first: when, the task, the model, the cost and how it ended.\nomega logs view <#> prints one; --tools keeps only the tool calls and what they returned.",
		"Lista las transcripciones de ejecuciones anteriores, la más reciente primero: cuándo, la tarea, el modelo, el coste y cómo acabó.\nomega logs view <#> muestra una; --tools deja solo las llamadas a herramientas y lo que devolvieron.",
		"Liste les transcriptions des exécutions précédentes, la plus récente d'abord : quand, la tâche, le modèle, le coût et l'issue.\nomega logs view <#> en affiche une ; --tools ne garde que les appels d'outils et leurs résultats.",
		"Listet die Protokolle früherer Läufe, neueste zuerst: wann, Aufgabe, Modell, Kosten und Ausgang.\nomega logs view <#> zeigt eines; --tools zeigt nur die Tool-Aufrufe und ihre Ergebnisse.",
		"Lista as transcrições das execuções anteriores, a mais recente primeiro: quando, a tarefa, o modelo, o custo e como acabou.\nomega logs view <#> mostra uma; --tools deixa só as chamadas a ferramentas e o que devolveram.",
	},
	"flag.logs_last":   {"show only the latest N runs", "mostrar solo las N ejecuciones más recientes", "n'afficher que les N dernières exécutions", "nur die letzten N Läufe zeigen", "mostrar só as N execuções mais recentes"},
	"flag.logs_task":   {"only the runs of this task", "solo las ejecuciones de esta tarea", "seulement les exécutions de cette tâche", "nur die Läufe dieser Aufgabe", "só as execuções desta tarefa"},
	"flag.logs_failed": {"only the runs that failed or did not finish", "solo las ejecuciones que fallaron o no terminaron", "seulement les exécutions en échec ou inachevées", "nur fehlgeschlagene oder unbeendete Läufe", "só as execuções que falharam ou não terminaram"},
	"flag.logs_tools":  {"show only the tool calls and their results", "mostrar solo las llamadas a herramientas y sus resultados", "n'afficher que les appels d'outils et leurs résultats", "nur Tool-Aufrufe und ihre Ergebnisse zeigen", "mostrar só as chamadas a ferramentas e os resultados"},
	"logs.none_match":  {"No run matches.", "Ninguna ejecución coincide.", "Aucune exécution ne correspond.", "Kein Lauf passt.", "Nenhuma execução corresponde."},

	// ---- pending, model, history ----
	"cmd.pending_long": {
		"What earlier runs found and left for you to do by hand (a setting to change, a permission to grant).\nRuns stop repeating an item once you dismiss it, snooze it or mark it done.",
		"Lo que las ejecuciones anteriores encontraron y te dejaron por hacer a mano (un ajuste, un permiso).\nLas ejecuciones dejan de repetirlo cuando lo descartas, lo pospones o lo marcas como hecho.",
		"Ce que les exécutions précédentes ont trouvé et vous ont laissé à faire (un réglage, une autorisation).\nElles cessent de le répéter quand vous l'écartez, le reportez ou le marquez fait.",
		"Was frühere Läufe gefunden und dir zum Erledigen übrig gelassen haben (eine Einstellung, eine Berechtigung).\nLäufe wiederholen es nicht mehr, sobald du es verwirfst, verschiebst oder erledigst.",
		"O que as execuções anteriores encontraram e deixaram para fazeres à mão (uma definição, uma permissão).\nDeixam de o repetir quando o descartas, adias ou marcas como feito.",
	},
	"cmd.model_long": {
		"With no argument, a picker of the provider's models. With a name, that model is fixed; with auto, omega\nchooses a cheap model and a stronger one for failures. omega model list shows the models with score and price.",
		"Sin argumento, un selector con los modelos del proveedor. Con un nombre, ese modelo queda fijo; con auto, omega\nelige uno barato y otro más potente para los fallos. omega model list muestra los modelos con nota y precio.",
		"Sans argument, un sélecteur des modèles du fournisseur. Avec un nom, ce modèle est fixé ; avec auto, omega\nchoisit un modèle économique et un plus puissant pour les échecs. omega model list montre les modèles, note et prix.",
		"Ohne Argument eine Auswahl der Modelle des Anbieters. Mit einem Namen wird dieses Modell fest; mit auto wählt omega\nein günstiges und ein stärkeres für Fehler. omega model list zeigt die Modelle mit Bewertung und Preis.",
		"Sem argumento, um seletor com os modelos do fornecedor. Com um nome, esse modelo fica fixo; com auto, o omega\nescolhe um barato e outro mais potente para as falhas. omega model list mostra os modelos com nota e preço.",
	},
	"cmd.history": {"Recent runs, open pending items and notes", "Ejecuciones recientes, pendientes y notas", "Exécutions récentes, points en attente et notes", "Letzte Läufe, offene Punkte und Notizen", "Execuções recentes, pendentes e notas"},
	"cmd.history_long": {
		"One screen with what omega remembers: the latest runs (omega logs), what is still pending (omega pending)\nand the newest notes (omega notes). Filter by task and by date with --task and --since.",
		"Una pantalla con lo que omega recuerda: las últimas ejecuciones (omega logs), lo que sigue pendiente (omega pending)\ny las notas más recientes (omega notes). Filtra por tarea y fecha con --task y --since.",
		"Un écran avec ce dont omega se souvient : les dernières exécutions (omega logs), ce qui reste en attente (omega pending)\net les notes récentes (omega notes). Filtrez par tâche et par date avec --task et --since.",
		"Ein Bildschirm mit dem, was omega weiß: die letzten Läufe (omega logs), was offen ist (omega pending)\nund die neuesten Notizen (omega notes). Mit --task und --since nach Aufgabe und Datum filtern.",
		"Um ecrã com o que o omega lembra: as últimas execuções (omega logs), o que continua pendente (omega pending)\ne as notas mais recentes (omega notes). Filtra por tarefa e data com --task e --since.",
	},
	"flag.history_last":  {"how many runs and notes to show", "cuántas ejecuciones y notas mostrar", "combien d'exécutions et de notes afficher", "wie viele Läufe und Notizen zeigen", "quantas execuções e notas mostrar"},
	"flag.history_since": {"since a date (2026-11-01) or for a span (7d, 2w, 12h)", "desde una fecha (2026-11-01) o un plazo (7d, 2w, 12h)", "depuis une date (2026-11-01) ou une durée (7d, 2w, 12h)", "seit einem Datum (2026-11-01) oder Zeitraum (7d, 2w, 12h)", "desde uma data (2026-11-01) ou um prazo (7d, 2w, 12h)"},
	"history.bad_since":  {"--since %s: give a date (2026-11-01) or a span (7d, 2w, 12h)", "--since %s: indica una fecha (2026-11-01) o un plazo (7d, 2w, 12h)", "--since %s : donnez une date (2026-11-01) ou une durée (7d, 2w, 12h)", "--since %s: ein Datum (2026-11-01) oder einen Zeitraum (7d, 2w, 12h) angeben", "--since %s: indica uma data (2026-11-01) ou um prazo (7d, 2w, 12h)"},
	"history.subtitle":   {"runs, pending items and notes", "ejecuciones, pendientes y notas", "exécutions, points en attente et notes", "Läufe, offene Punkte und Notizen", "execuções, pendentes e notas"},
	"history.runs":       {"Recent runs", "Ejecuciones recientes", "Exécutions récentes", "Letzte Läufe", "Execuções recentes"},
	"history.pending":    {"Pending", "Pendiente", "En attente", "Offen", "Pendente"},
	"history.notes":      {"Latest notes", "Últimas notas", "Dernières notes", "Neueste Notizen", "Últimas notas"},
	"history.hint": {
		"More: omega logs · omega pending --all · omega notes", "Más: omega logs · omega pending --all · omega notes", "Plus : omega logs · omega pending --all · omega notes",
		"Mehr: omega logs · omega pending --all · omega notes", "Mais: omega logs · omega pending --all · omega notes",
	},

	// ---- notes compact ----
	"cmd.notes_compact": {"Remove notes that repeat a newer one", "Quitar las notas que repiten otra más reciente", "Retirer les notes qui en répètent une plus récente", "Notizen entfernen, die eine neuere wiederholen", "Tirar as notas que repetem outra mais recente"},
	"cmd.notes_compact_long": {
		"Finds the notes that say the same as a newer one (the same words once dates, case and punctuation are\nleft aside, or nearly all of them in a long note), shows them and removes them after a yes. No AI is used.\nThe previous file is kept as notes.md.bak.",
		"Busca las notas que dicen lo mismo que otra más reciente (las mismas palabras sin contar fechas, mayúsculas ni\npuntuación, o casi todas en una nota larga), las muestra y las quita tras un sí. No usa IA.\nEl fichero anterior queda como notes.md.bak.",
		"Cherche les notes qui disent la même chose qu'une plus récente (les mêmes mots, hors dates, casse et ponctuation,\nou presque tous dans une note longue), les montre et les retire après un oui. Sans IA.\nL'ancien fichier reste en notes.md.bak.",
		"Sucht Notizen, die dasselbe sagen wie eine neuere (dieselben Wörter ohne Datum, Groß-/Kleinschreibung und\nSatzzeichen, oder fast alle in einer langen Notiz), zeigt sie und entfernt sie nach einem Ja. Ohne KI.\nDie alte Datei bleibt als notes.md.bak.",
		"Procura as notas que dizem o mesmo que outra mais recente (as mesmas palavras sem contar datas, maiúsculas e\npontuação, ou quase todas numa nota longa), mostra-as e tira-as após um sim. Sem IA.\nO ficheiro anterior fica como notes.md.bak.",
	},
	"flag.compact_yes": {"remove them without asking", "quitarlas sin preguntar", "les retirer sans demander", "ohne Rückfrage entfernen", "tirá-las sem perguntar"},
	"flag.compact_dry": {"only show what would be removed", "solo mostrar lo que se quitaría", "montrer seulement ce qui serait retiré", "nur zeigen, was entfernt würde", "só mostrar o que se tiraria"},
	"notes.no_repeats": {"No repeated notes.", "No hay notas repetidas.", "Aucune note répétée.", "Keine wiederholten Notizen.", "Não há notas repetidas."},
	"notes.same_as":    {"the same as line %d, which stays", "igual que la línea %d, que se queda", "comme la ligne %d, qui reste", "wie Zeile %d, die bleibt", "igual à linha %d, que fica"},
	"notes.would_remove": {
		"%d of %d notes repeat a newer one.", "%d de %d notas repiten otra más reciente.", "%d notes sur %d en répètent une plus récente.",
		"%d von %d Notizen wiederholen eine neuere.", "%d de %d notas repetem outra mais recente.",
	},
	"notes.would_remove.one": {
		"%d of %d notes repeats a newer one.", "%d de %d notas repite otra más reciente.", "%d note sur %d en répète une plus récente.",
		"%d von %d Notizen wiederholt eine neuere.", "%d de %d notas repete outra mais recente.",
	},
	"notes.compact_ask": {"Remove them? y/N", "¿Quitarlas? s/N", "Les retirer ? o/N", "Entfernen? j/N", "Tirá-las? s/N"},
	"notes.compact_need_yes": {
		"nothing was removed: confirm at a terminal or pass --yes", "no se quitó nada: confirma en un terminal o usa --yes", "rien n'a été retiré : confirmez dans un terminal ou passez --yes",
		"nichts entfernt: am Terminal bestätigen oder --yes angeben", "nada foi tirado: confirma num terminal ou usa --yes",
	},
	"notes.compacted": {
		"Removed %d notes; the previous file is %s", "Quitadas %d notas; el fichero anterior es %s", "%d notes retirées ; l'ancien fichier est %s",
		"%d Notizen entfernt; die alte Datei ist %s", "Tiradas %d notas; o ficheiro anterior é %s",
	},
	"notes.compacted.one": {
		"Removed %d note; the previous file is %s", "Quitada %d nota; el fichero anterior es %s", "%d note retirée ; l'ancien fichier est %s",
		"%d Notiz entfernt; die alte Datei ist %s", "Tirada %d nota; o ficheiro anterior é %s",
	},

	// ---- steps ----
	"cmd.steps_show":       {"Everything about one step: command, risk, install", "Todo sobre un paso: orden, riesgo, instalación", "Tout sur une étape : commande, risque, installation", "Alles zu einem Schritt: Befehl, Risiko, Installation", "Tudo sobre um passo: comando, risco, instalação"},
	"flag.steps_installed": {"only the steps installed here", "solo los pasos instalados aquí", "seulement les étapes installées ici", "nur die hier installierten Schritte", "só os passos instalados aqui"},
	"flag.steps_category":  {"only one category: check, update, clean or backend", "solo una categoría: check, update, clean o backend", "une seule catégorie : check, update, clean ou backend", "nur eine Kategorie: check, update, clean oder backend", "só uma categoria: check, update, clean ou backend"},
	"steps.bad_category":   {"no category %q: use one of %s", "no hay categoría %q: usa una de %s", "pas de catégorie %q : utilisez l'une de %s", "keine Kategorie %q: nimm eine von %s", "não há categoria %q: usa uma de %s"},
	"steps.f_name":         {"name", "nombre", "nom", "Name", "nome"},
	"steps.f_category":     {"category", "categoría", "catégorie", "Kategorie", "categoria"},
	"steps.f_risk":         {"risk", "riesgo", "risque", "Risiko", "risco"},
	"steps.f_state":        {"here", "aquí", "ici", "hier", "aqui"},
	"steps.f_install":      {"install", "instalar", "installer", "installieren", "instalar"},
	"steps.f_source":       {"from", "origen", "origine", "Herkunft", "origem"},
	"steps.f_what":         {"What it does", "Qué hace", "Ce qu'elle fait", "Was er tut", "O que faz"},
	"steps.f_command":      {"Command", "Orden", "Commande", "Befehl", "Comando"},
	"steps.f_last":         {"Last runs (omega custom)", "Últimas ejecuciones (omega custom)", "Dernières exécutions (omega custom)", "Letzte Läufe (omega custom)", "Últimas execuções (omega custom)"},
	"steps.always_asks":    {"always asks, even with --yes", "pregunta siempre, incluso con --yes", "demande toujours, même avec --yes", "fragt immer, auch mit --yes", "pergunta sempre, mesmo com --yes"},
	"steps.installed":      {"installed", "instalado", "installée", "installiert", "instalado"},
	"steps.missing":        {"not installed (needs %s)", "no instalado (necesita %s)", "non installée (demande %s)", "nicht installiert (braucht %s)", "não instalado (precisa de %s)"},
	"steps.own":            {"your steps.d", "tu steps.d", "votre steps.d", "dein steps.d", "o teu steps.d"},
	"steps.show_hint": {
		"Run it: omega steps run %[1]s  ·  try it: omega steps run %[1]s --dry-run", "Ejecútalo: omega steps run %[1]s  ·  pruébalo: omega steps run %[1]s --dry-run",
		"L'exécuter : omega steps run %[1]s  ·  l'essayer : omega steps run %[1]s --dry-run", "Ausführen: omega steps run %[1]s  ·  testen: omega steps run %[1]s --dry-run",
		"Executa-o: omega steps run %[1]s  ·  experimenta: omega steps run %[1]s --dry-run",
	},

	// ---- named routines ----
	"cmd.custom_long2": {
		"You may keep several routines with names: omega custom --set weekly brew mole-clean saves one, omega custom weekly\nruns it, --list shows them and omega custom weekly --default makes it the one omega custom runs. Per routine,\n--stop-on-fail ID… ends the run when one of those steps fails, and --only-below-free-gb N runs it only when the\ndisk has less free space. omega schedule install --task custom runs the default routine on a schedule.",
		"Puedes tener varias rutinas con nombre: omega custom --set semanal brew mole-clean guarda una, omega custom semanal\nla ejecuta, --list las muestra y omega custom semanal --default la hace la que ejecuta omega custom. Por rutina,\n--stop-on-fail ID… para la ejecución si falla uno de esos pasos, y --only-below-free-gb N solo la ejecuta cuando\nel disco tiene menos espacio libre. omega schedule install --task custom programa la rutina por defecto.",
		"Vous pouvez garder plusieurs routines nommées : omega custom --set hebdo brew mole-clean en enregistre une, omega custom hebdo\nl'exécute, --list les montre et omega custom hebdo --default en fait celle que lance omega custom. Par routine,\n--stop-on-fail ID… arrête l'exécution si l'une de ces étapes échoue, et --only-below-free-gb N ne la lance que si le\ndisque a moins d'espace libre. omega schedule install --task custom programme la routine par défaut.",
		"Du kannst mehrere benannte Routinen haben: omega custom --set woche brew mole-clean speichert eine, omega custom woche\nführt sie aus, --list zeigt sie und omega custom woche --default macht sie zu der, die omega custom ausführt. Pro Routine\nbeendet --stop-on-fail ID… den Lauf, wenn einer dieser Schritte fehlschlägt, und --only-below-free-gb N lässt sie nur\nlaufen, wenn die Platte weniger frei hat. omega schedule install --task custom plant die Standardroutine ein.",
		"Podes ter várias rotinas com nome: omega custom --set semanal brew mole-clean guarda uma, omega custom semanal\nexecuta-a, --list mostra-as e omega custom semanal --default torna-a a que omega custom executa. Por rotina,\n--stop-on-fail ID… para a execução se um desses passos falhar, e --only-below-free-gb N só a executa quando o\ndisco tem menos espaço livre. omega schedule install --task custom agenda a rotina por omissão.",
	},
	"flag.custom_list":     {"list the routines", "listar las rutinas", "lister les routines", "die Routinen auflisten", "listar as rotinas"},
	"flag.custom_default":  {"make a routine the one omega custom runs", "hacer de una rutina la que ejecuta omega custom", "faire d'une routine celle que lance omega custom", "eine Routine zur Standardroutine machen", "tornar uma rotina a que omega custom executa"},
	"flag.custom_stop":     {"with --set: steps whose failure ends the run", "con --set: pasos cuyo fallo para la ejecución", "avec --set : étapes dont l'échec arrête l'exécution", "mit --set: Schritte, deren Fehler den Lauf beendet", "com --set: passos cuja falha para a execução"},
	"flag.custom_below":    {"with --set: run only below this many GB free (0: always)", "con --set: ejecutar solo con menos GB libres (0: siempre)", "avec --set : seulement sous ce nombre de Go libres (0 : toujours)", "mit --set: nur unter so vielen GB frei (0: immer)", "com --set: só com menos GB livres (0: sempre)"},
	"custom.title_named":   {"routine %s", "rutina %s", "routine %s", "Routine %s", "rotina %s"},
	"custom.cleared_one":   {"Routine %s deleted.", "Rutina %s borrada.", "Routine %s supprimée.", "Routine %s gelöscht.", "Rotina %s apagada."},
	"custom.default_set":   {"omega custom now runs the routine %s.", "omega custom ejecuta ahora la rutina %s.", "omega custom lance désormais la routine %s.", "omega custom führt jetzt die Routine %s aus.", "omega custom executa agora a rotina %s."},
	"custom.options_saved": {"Options of the routine %s saved.", "Opciones de la rutina %s guardadas.", "Options de la routine %s enregistrées.", "Optionen der Routine %s gespeichert.", "Opções da rotina %s guardadas."},
	"custom.no_routine":    {"there is no routine %q (there are: %s)", "no hay ninguna rutina %q (hay: %s)", "il n'y a pas de routine %q (il y a : %s)", "es gibt keine Routine %q (vorhanden: %s)", "não há nenhuma rotina %q (há: %s)"},
	"custom.no_args":       {"%s takes no step or routine", "%s no lleva pasos ni rutina", "%s ne prend ni étape ni routine", "%s nimmt weder Schritt noch Routine", "%s não leva passos nem rotina"},
	"custom.extra_args": {
		"unexpected words %q: to change a routine use omega custom --set %s", "palabras inesperadas %q: para cambiar una rutina usa omega custom --set %s",
		"mots inattendus %q : pour changer une routine, utilisez omega custom --set %s", "unerwartete Wörter %q: eine Routine ändert omega custom --set %s",
		"palavras inesperadas %q: para mudar uma rotina usa omega custom --set %s",
	},
	"custom.is_default":  {"default", "por defecto", "par défaut", "Standard", "por omissão"},
	"custom.col_routine": {"routine", "rutina", "routine", "Routine", "rotina"},
	"custom.col_steps":   {"steps", "pasos", "étapes", "Schritte", "passos"},
	"custom.n_steps":     {"%d steps", "%d pasos", "%d étapes", "%d Schritte", "%d passos"},
	"custom.n_steps.one": {"%d step", "%d paso", "%d étape", "%d Schritt", "%d passo"},
	"custom.opt_below":   {"only below %g GB free", "solo con menos de %g GB libres", "seulement sous %g Go libres", "nur unter %g GB frei", "só com menos de %g GB livres"},
	"custom.opt_stop":    {"stops if %s fails", "para si falla %s", "s'arrête si %s échoue", "stoppt, wenn %s fehlschlägt", "para se %s falhar"},
	"custom.stops":       {"stops the run if it fails", "para la rutina si falla", "arrête la routine en cas d'échec", "stoppt die Routine bei Fehler", "para a rotina se falhar"},
	"custom.stopped":     {"%s failed and ends the routine: the steps after it did not run.", "%s falló y termina la rutina: los pasos siguientes no se ejecutaron.", "%s a échoué et termine la routine : les étapes suivantes n'ont pas été exécutées.", "%s ist fehlgeschlagen und beendet die Routine: die Schritte danach liefen nicht.", "%s falhou e termina a rotina: os passos seguintes não foram executados."},
	"custom.enough_space": {
		"%s free, not under %.0f GB: the routine has nothing to do.", "%s libres, no menos de %.0f GB: la rutina no tiene nada que hacer.",
		"%s libres, pas moins de %.0f Go : la routine n'a rien à faire.", "%s frei, nicht unter %.0f GB: die Routine hat nichts zu tun.",
		"%s livres, não menos de %.0f GB: a rotina não tem nada a fazer.",
	},
	"custom.hint_list": {
		"Run one: omega custom <name>  ·  make it the default: omega custom <name> --default", "Ejecuta una: omega custom <nombre>  ·  por defecto: omega custom <nombre> --default",
		"En lancer une : omega custom <nom>  ·  par défaut : omega custom <nom> --default", "Eine ausführen: omega custom <name>  ·  als Standard: omega custom <name> --default",
		"Executa uma: omega custom <nome>  ·  por omissão: omega custom <nome> --default",
	},

	// ---- completion and man ----
	"cmd.completion": {"Completion of commands and options for your shell", "Autocompletado de órdenes y opciones para tu shell", "Complétion des commandes et options pour votre shell", "Vervollständigung von Befehlen und Optionen", "Autocompletar comandos e opções na tua shell"},
	"cmd.completion_long": {
		"Prints the completion script for bash, zsh, fish or PowerShell, with a description for each command, and\ncompletes values too: providers, models, step ids, pending ids, run numbers. omega completion install puts\nthe script where your shell reads it.",
		"Escribe el script de autocompletado para bash, zsh, fish o PowerShell, con una descripción de cada orden, y\ncompleta también valores: proveedores, modelos, ids de pasos y pendientes, números de ejecución.\nomega completion install lo deja donde tu shell lo lee.",
		"Écrit le script de complétion pour bash, zsh, fish ou PowerShell, avec une description de chaque commande, et\ncomplète aussi les valeurs : fournisseurs, modèles, ids d'étapes et de points en attente, numéros d'exécution.\nomega completion install le met là où votre shell le lit.",
		"Gibt das Vervollständigungsskript für bash, zsh, fish oder PowerShell aus, mit Beschreibung je Befehl, und\nergänzt auch Werte: Anbieter, Modelle, Schritt- und Offen-IDs, Laufnummern. omega completion install legt es\ndorthin, wo deine Shell es liest.",
		"Escreve o script de autocompletar para bash, zsh, fish ou PowerShell, com uma descrição de cada comando, e\ncompleta também valores: fornecedores, modelos, ids de passos e pendentes, números de execução.\nomega completion install coloca-o onde a tua shell o lê.",
	},
	"cmd.completion_shell":   {"Print the %s completion script", "Escribir el script de autocompletado de %s", "Écrire le script de complétion %s", "Das %s-Vervollständigungsskript ausgeben", "Escrever o script de autocompletar de %s"},
	"cmd.completion_install": {"Install the completion for your shell", "Instalar el autocompletado de tu shell", "Installer la complétion pour votre shell", "Vervollständigung für deine Shell installieren", "Instalar o autocompletar da tua shell"},
	"completion.zsh_tip": {
		"Tip: ~/.zshrc needs fpath=(~/.zfunc $fpath) and autoload -Uz compinit && compinit",
		"Consejo: ~/.zshrc necesita fpath=(~/.zfunc $fpath) y autoload -Uz compinit && compinit",
		"Astuce : ~/.zshrc doit contenir fpath=(~/.zfunc $fpath) et autoload -Uz compinit && compinit",
		"Tipp: ~/.zshrc braucht fpath=(~/.zfunc $fpath) und autoload -Uz compinit && compinit",
		"Dica: ~/.zshrc precisa de fpath=(~/.zfunc $fpath) e autoload -Uz compinit && compinit",
	},
	"cmd.man": {"Show omega's man page: every command and option", "Ver la página man de omega: órdenes y opciones", "Voir la page man d'omega : commandes et options", "omegas Manpage zeigen: alle Befehle und Optionen", "Ver a página man do omega: comandos e opções"},
	"cmd.man_long": {
		"Shows the man page (omega.1) with man at a terminal, and prints its troff source anywhere else\n(omega man > omega.1). --output DIR writes DIR/omega.1. The page is in English in every language.",
		"Muestra la página man (omega.1) con man en un terminal, y escribe su fuente troff en cualquier otro caso\n(omega man > omega.1). --output DIR escribe DIR/omega.1. La página está en inglés en todos los idiomas.",
		"Affiche la page man (omega.1) avec man dans un terminal, et écrit sa source troff sinon\n(omega man > omega.1). --output DIR écrit DIR/omega.1. La page est en anglais dans toutes les langues.",
		"Zeigt die Manpage (omega.1) am Terminal mit man und gibt sonst ihre troff-Quelle aus\n(omega man > omega.1). --output DIR schreibt DIR/omega.1. Die Seite ist in jeder Sprache englisch.",
		"Mostra a página man (omega.1) com man num terminal, e escreve a fonte troff nos outros casos\n(omega man > omega.1). --output DIR escreve DIR/omega.1. A página está em inglês em todas as línguas.",
	},
	"flag.man_output": {"write omega.1 into this folder instead of showing it", "escribir omega.1 en esta carpeta en vez de mostrarla", "écrire omega.1 dans ce dossier au lieu de l'afficher", "omega.1 in diesen Ordner schreiben statt sie zu zeigen", "escrever omega.1 nesta pasta em vez de a mostrar"},
	"man.written":     {"Man page written: %s", "Página man escrita: %s", "Page man écrite : %s", "Manpage geschrieben: %s", "Página man escrita: %s"},
	"config.unknown_key": {
		"config.toml: unknown key %q, ignored (a typo? omega config show lists the keys)", "config.toml: clave desconocida %q, se ignora (¿una errata? omega config show lista las claves)",
		"config.toml : clé inconnue %q, ignorée (une coquille ? omega config show liste les clés)", "config.toml: unbekannter Schlüssel %q, ignoriert (Tippfehler? omega config show listet die Schlüssel)",
		"config.toml: chave desconhecida %q, ignorada (uma gralha? omega config show lista as chaves)",
	},

	// ---- the longer description of commands whose Short became shorter ----
	"cmd.auth_more": {
		"Manage API keys, kept where the system keeps secrets (macOS Keychain, Windows Credential Manager, Linux keyring or a private file)",
		"Gestionar las API keys, guardadas donde el sistema guarda secretos (llavero de macOS, Administrador de credenciales de Windows, llavero de Linux o un archivo privado)",
		"Gérer les clés API, gardées là où le système garde ses secrets (trousseau macOS, Gestionnaire d'identification de Windows, trousseau Linux ou fichier privé)",
		"API-Keys verwalten, abgelegt wo das System Geheimnisse aufbewahrt (macOS-Schlüsselbund, Windows-Anmeldeinformationsverwaltung, Linux-Schlüsselbund oder private Datei)",
		"Gerir as API keys, guardadas onde o sistema guarda segredos (porta-chaves do macOS, Gestor de Credenciais do Windows, porta-chaves do Linux ou um ficheiro privado)",
	},
	"cmd.config_set_more": {
		"Change a value: provider, model, effort, approval, language, command_timeout, skip_steps",
		"Cambiar un valor: provider, model, effort, approval, language, command_timeout, skip_steps",
		"Modifier une valeur : provider, model, effort, approval, language, command_timeout, skip_steps",
		"Einen Wert ändern: provider, model, effort, approval, language, command_timeout, skip_steps",
		"Alterar um valor: provider, model, effort, approval, language, command_timeout, skip_steps",
	},
	"cmd.dev_activate_more": {
		"Add the shell lines installed tools still need to ~/.zshrc or ~/.bashrc",
		"Añade a ~/.zshrc o ~/.bashrc las líneas que aún necesitan las herramientas instaladas",
		"Ajouter à ~/.zshrc ou ~/.bashrc les lignes dont les outils installés ont encore besoin",
		"Shell-Zeilen, die installierte Werkzeuge noch brauchen, in ~/.zshrc oder ~/.bashrc eintragen",
		"Acrescenta a ~/.zshrc ou ~/.bashrc as linhas de que as ferramentas instaladas ainda precisam",
	},
	"cmd.dev_status_more": {
		"Installed languages and tools, what installed each one, and what stands out (no AI)",
		"Lenguajes y herramientas instalados, quién instaló cada uno y qué llama la atención (sin IA)",
		"Langages et outils installés, ce qui a installé chacun et ce qui ressort (sans IA)",
		"Installierte Sprachen und Werkzeuge, wer sie installiert hat und was auffällt (ohne KI)",
		"Linguagens e ferramentas instaladas, quem instalou cada uma e o que chama a atenção (sem IA)",
	},
	"cmd.doctor_more": {
		"Check config, key, provider and installed steps without spending tokens",
		"Comprobar configuración, clave, proveedor y pasos instalados sin gastar tokens",
		"Vérifier la configuration, la clé, le fournisseur et les étapes installées sans dépenser de jetons",
		"Konfiguration, Schlüssel, Anbieter und installierte Schritte prüfen, ohne Tokens zu verbrauchen",
		"Verificar a configuração, a chave, o fornecedor e os passos instalados sem gastar tokens",
	},
	"cmd.mac_apps_more": {
		"Review apps you no longer open, Intel-only apps and what removed apps left behind",
		"Revisa las apps que ya no abres, las que solo son Intel y los restos de apps eliminadas",
		"Passer en revue les apps que vous n'ouvrez plus, les apps Intel et les restes d'apps supprimées",
		"Apps prüfen, die du nicht mehr öffnest, reine Intel-Apps und Reste entfernter Apps",
		"Revê as apps que já não abres, as que são só Intel e os restos de apps removidas",
	},
	"cmd.mac_permissions_more": {
		"Remove privacy permissions left behind by binaries and apps that are gone",
		"Quita los permisos de privacidad que dejaron binarios y apps que ya no existen",
		"Retirer les autorisations de confidentialité laissées par des binaires et apps disparus",
		"Datenschutz-Berechtigungen entfernen, die verschwundene Programme und Apps hinterlassen haben",
		"Remove as permissões de privacidade deixadas por binários e apps que já não existem",
	},
	"cmd.setup_more": {
		"Wizard: pick a provider, store the API key and choose a model",
		"Asistente: elegir proveedor, guardar la API key y escoger modelo",
		"Assistant : choisir un fournisseur, enregistrer la clé API et choisir un modèle",
		"Assistent: Anbieter wählen, API-Key speichern und Modell auswählen",
		"Assistente: escolher fornecedor, guardar a API key e escolher modelo",
	},
	"cmd.docs_search": {"Search the manual, offline", "Buscar en el manual, sin conexión", "Chercher dans le manuel, hors ligne", "Das Handbuch offline durchsuchen", "Pesquisar no manual, sem ligação"},
	"cmd.docs_ask":    {"Ask the manual; your model answers from it", "Preguntar al manual; responde tu modelo", "Interroger le manuel ; votre modèle répond", "Das Handbuch fragen; dein Modell antwortet", "Perguntar ao manual; responde o teu modelo"},

	// ---- names of the steps ----
	"step.brew-doctor":        {"Homebrew check (brew doctor)", "Revisión de Homebrew (brew doctor)", "Vérification Homebrew (brew doctor)", "Homebrew-Prüfung (brew doctor)", "Verificação do Homebrew (brew doctor)"},
	"step.dev-artifacts":      {"Stale project build folders", "Carpetas de compilación de proyectos antiguos", "Dossiers de build de projets anciens", "Alte Build-Ordner von Projekten", "Pastas de build de projetos antigos"},
	"step.shell-startup":      {"Shell startup time", "Tiempo de arranque de la shell", "Temps de démarrage du shell", "Startzeit der Shell", "Tempo de arranque da shell"},
	"step.uv":                 {"uv tools", "Herramientas de uv", "Outils uv", "uv-Tools", "Ferramentas do uv"},
	"step.rustup":             {"Rust toolchains", "Toolchains de Rust", "Chaînes d'outils Rust", "Rust-Toolchains", "Toolchains do Rust"},
	"step.mise":               {"mise tools", "Herramientas de mise", "Outils mise", "mise-Tools", "Ferramentas do mise"},
	"step.asdf":               {"asdf plugins", "Plugins de asdf", "Plugins asdf", "asdf-Plugins", "Plugins do asdf"},
	"step.composer":           {"Composer global packages", "Paquetes globales de Composer", "Paquets Composer globaux", "Globale Composer-Pakete", "Pacotes globais do Composer"},
	"step.go-bins":            {"Go binaries", "Binarios de Go", "Binaires Go", "Go-Programme", "Binários do Go"},
	"step.dotnet-tools":       {".NET global tools", "Herramientas globales de .NET", "Outils .NET globaux", "Globale .NET-Tools", "Ferramentas globais do .NET"},
	"step.vscode-ext":         {"VS Code extensions", "Extensiones de VS Code", "Extensions VS Code", "VS-Code-Erweiterungen", "Extensões do VS Code"},
	"step.tldr":               {"tldr pages", "Páginas de tldr", "Pages tldr", "tldr-Seiten", "Páginas do tldr"},
	"step.helm-repos":         {"Helm repositories", "Repositorios de Helm", "Dépôts Helm", "Helm-Repositories", "Repositórios do Helm"},
	"step.gcloud":             {"Google Cloud CLI components", "Componentes de Google Cloud CLI", "Composants de Google Cloud CLI", "Google-Cloud-CLI-Komponenten", "Componentes do Google Cloud CLI"},
	"step.tmux-plugins":       {"tmux plugins", "Plugins de tmux", "Plugins tmux", "tmux-Plugins", "Plugins do tmux"},
	"step.nvim-plugins":       {"Neovim plugins (lazy.nvim)", "Plugins de Neovim (lazy.nvim)", "Plugins Neovim (lazy.nvim)", "Neovim-Plugins (lazy.nvim)", "Plugins do Neovim (lazy.nvim)"},
	"step.go-modcache":        {"Go module cache", "Caché de módulos de Go", "Cache des modules Go", "Go-Modul-Cache", "Cache de módulos do Go"},
	"step.yarn-cache":         {"Yarn cache", "Caché de Yarn", "Cache Yarn", "Yarn-Cache", "Cache do Yarn"},
	"step.bun-cache":          {"Bun cache", "Caché de Bun", "Cache Bun", "Bun-Cache", "Cache do Bun"},
	"step.deno-cache":         {"Deno cache", "Caché de Deno", "Cache Deno", "Deno-Cache", "Cache do Deno"},
	"step.cargo-cache":        {"Cargo registry cache", "Caché del registro de Cargo", "Cache du registre Cargo", "Cargo-Registry-Cache", "Cache do registo do Cargo"},
	"step.gem-cleanup":        {"Old gem versions", "Versiones antiguas de gems", "Anciennes versions des gems", "Alte Gem-Versionen", "Versões antigas de gems"},
	"step.composer-cache":     {"Composer cache", "Caché de Composer", "Cache Composer", "Composer-Cache", "Cache do Composer"},
	"step.cocoapods-cache":    {"CocoaPods cache", "Caché de CocoaPods", "Cache CocoaPods", "CocoaPods-Cache", "Cache do CocoaPods"},
	"step.gradle-cache":       {"Gradle caches", "Cachés de Gradle", "Caches Gradle", "Gradle-Caches", "Caches do Gradle"},
	"step.conda-clean":        {"conda caches", "Cachés de conda", "Caches conda", "conda-Caches", "Caches do conda"},
	"step.mise-prune":         {"Unused mise versions", "Versiones de mise sin usar", "Versions mise inutilisées", "Unbenutzte mise-Versionen", "Versões do mise sem uso"},
	"step.nuget-cache":        {"NuGet caches", "Cachés de NuGet", "Caches NuGet", "NuGet-Caches", "Caches do NuGet"},
	"step.flutter-cache":      {"Dart pub cache", "Caché de pub de Dart", "Cache pub de Dart", "Dart-pub-Cache", "Cache do pub do Dart"},
	"step.pre-commit-gc":      {"pre-commit environments", "Entornos de pre-commit", "Environnements pre-commit", "pre-commit-Umgebungen", "Ambientes do pre-commit"},
	"step.ios-device-support": {"Xcode device support files", "Archivos de soporte de dispositivos de Xcode", "Fichiers de prise en charge des appareils Xcode", "Xcode-Gerätesupportdateien", "Ficheiros de suporte de dispositivos do Xcode"},
	"step.xcode-derived":      {"Xcode DerivedData", "DerivedData de Xcode", "DerivedData de Xcode", "Xcode-DerivedData", "DerivedData do Xcode"},
	"step.mole-clean":         {"Mole cleanup", "Limpieza de Mole", "Nettoyage Mole", "Mole-Bereinigung", "Limpeza do Mole"},
	"step.mole-optimize":      {"Mole optimization", "Optimización de Mole", "Optimisation Mole", "Mole-Optimierung", "Otimização do Mole"},
}

func init() {
	for k, v := range cliMessages {
		messages[k] = v
	}
}
