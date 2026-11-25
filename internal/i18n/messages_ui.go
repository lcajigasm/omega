package i18n

// Messages of the terminal interface's shared key line ("key action · key
// action"), and the texts the screens added with it. Keys and actions are
// separate so the key can be drawn in bold. Same order as messages: en, es,
// fr, de, pt.
var uiMessages = map[string][]string{
	// Names of keys.
	"uix.k_enter": {"enter", "enter", "entrée", "Enter", "enter"},
	"uix.k_esc":   {"esc", "esc", "échap", "Esc", "esc"},
	"uix.k_space": {"space", "espacio", "espace", "Leertaste", "espaço"},
	"uix.k_ctrl":  {"ctrl", "ctrl", "ctrl", "Strg", "ctrl"},
	"uix.k_shift": {"shift", "shift", "maj", "Shift", "shift"},
	"uix.k_drag":  {"drag", "arrastrar", "glisser", "ziehen", "arrastar"},
	"uix.k_up":    {"up", "arriba", "haut", "hoch", "cima"},
	"uix.k_down":  {"down", "abajo", "bas", "runter", "baixo"},

	// Actions of the session.
	"uix.cancel":      {"cancel", "cancelar", "annuler", "abbrechen", "cancelar"},
	"uix.help":        {"help", "ayuda", "aide", "Hilfe", "ajuda"},
	"uix.search":      {"search", "buscar", "chercher", "suchen", "procurar"},
	"uix.mode":        {"mode", "modo", "mode", "Modus", "modo"},
	"uix.model":       {"model", "modelo", "modèle", "Modell", "modelo"},
	"uix.scroll":      {"scroll", "desplazar", "défiler", "scrollen", "deslocar"},
	"uix.send":        {"send", "enviar", "envoyer", "senden", "enviar"},
	"uix.commands":    {"commands", "comandos", "commandes", "Befehle", "comandos"},
	"uix.esc_idle":    {"clear, again to quit", "borrar, otra vez sale", "effacer, encore pour quitter", "löschen, nochmal beendet", "limpar, outra vez sai"},
	"uix.quit":        {"quit", "salir", "quitter", "beenden", "sair"},
	"uix.leave_again": {"again to leave", "otra vez para salir", "encore pour quitter", "noch einmal zum Beenden", "outra vez para sair"},
	"uix.turn_cancelled": {
		"Stopped. Type what to do next, or press ctrl+c again to leave.", "Detenido. Escribe qué hacer ahora, o pulsa ctrl+c otra vez para salir.",
		"Arrêté. Écrivez la suite, ou appuyez encore sur ctrl+c pour quitter.", "Angehalten. Schreib, wie es weitergeht, oder drücke Strg+C noch einmal zum Beenden.",
		"Parado. Escreve o que fazer a seguir, ou prime ctrl+c outra vez para sair.",
	},
	"uix.cancelled": {
		"Cancelled. Press ctrl+c again to leave.", "Cancelado. Pulsa ctrl+c otra vez para salir.", "Annulé. Appuyez encore sur ctrl+c pour quitter.",
		"Abgebrochen. Drücke Strg+C noch einmal zum Beenden.", "Cancelado. Prima ctrl+c outra vez para sair.",
	},
	"uix.close_help":    {"close help", "cerrar la ayuda", "fermer l'aide", "Hilfe schließen", "fechar a ajuda"},
	"uix.next":          {"next", "siguiente", "suivant", "weiter", "seguinte"},
	"uix.previous":      {"previous", "anterior", "précédent", "zurück", "anterior"},
	"uix.close":         {"close", "cerrar", "fermer", "schließen", "fechar"},
	"uix.confirm":       {"confirm", "confirmar", "valider", "bestätigen", "confirmar"},
	"uix.skip_admin":    {"skip what needs admin access", "saltar lo que requiere permisos", "ignorer ce qui demande des droits", "Admin-Schritte überspringen", "saltar o que precisa de permissões"},
	"uix.expand":        {"expand", "expandir", "développer", "aufklappen", "expandir"},
	"uix.new_lines":     {"%d new lines", "%d líneas nuevas", "%d nouvelles lignes", "%d neue Zeilen", "%d linhas novas"},
	"uix.new_lines.one": {"%d new line", "%d línea nueva", "%d nouvelle ligne", "%d neue Zeile", "%d linha nova"},
	"uix.to_end":        {"go to the end", "ir al final", "aller à la fin", "zum Ende", "ir para o fim"},
	"uix.silent":        {"no output for %s", "sin salida desde hace %s", "aucune sortie depuis %s", "seit %s keine Ausgabe", "sem saída há %s"},
	"uix.select_text":   {"Select text (OMEGA_MOUSE=0 leaves the mouse to the terminal)", "Seleccionar texto (OMEGA_MOUSE=0 deja el ratón a la terminal)", "Sélectionner du texte (OMEGA_MOUSE=0 laisse la souris au terminal)", "Text auswählen (OMEGA_MOUSE=0 überlässt die Maus dem Terminal)", "Selecionar texto (OMEGA_MOUSE=0 deixa o rato ao terminal)"},

	// Short names of the approval modes, for the panel.
	"uix.mode_ask_short":  {"asks first", "pregunta antes", "demande d'abord", "fragt vorher", "pergunta antes"},
	"uix.mode_auto_short": {"auto approve", "aprueba solo", "approuve seul", "genehmigt selbst", "aprova sozinho"},
	"uix.mode_ro_short":   {"read only", "solo lectura", "lecture seule", "nur lesen", "só leitura"},

	// The approval card.
	"uix.yes":       {"yes", "sí", "oui", "ja", "sim"},
	"uix.yes_all":   {"yes to all", "sí a todo", "oui à tout", "ja zu allem", "sim a tudo"},
	"uix.no":        {"no", "no", "non", "nein", "não"},
	"uix.yes_key":   {"y", "s", "o", "j", "s"},
	"uix.enter_not": {"enter does not approve a dangerous command", "enter no aprueba un comando peligroso", "entrée n'approuve pas une commande dangereuse", "Enter genehmigt keinen gefährlichen Befehl", "enter não aprova um comando perigoso"},
	"uix.blocked":   {"Blocked", "Bloqueado", "Bloqué", "Blockiert", "Bloqueado"},

	// Lists.
	"uix.move":        {"move", "mover", "déplacer", "bewegen", "mover"},
	"uix.type_filter": {"type to filter", "escribe para filtrar", "tapez pour filtrer", "tippen zum Filtern", "escreva para filtrar"},
	"uix.choose":      {"choose", "elegir", "choisir", "wählen", "escolher"},
	"uix.select":      {"select", "marcar", "cocher", "wählen", "marcar"},
	"uix.all":         {"all", "todas", "tout", "alle", "todas"},
	"uix.confirm_n":   {"confirm (%d)", "confirmar (%d)", "valider (%d)", "bestätigen (%d)", "confirmar (%d)"},
	"uix.none":        {"none", "ninguna", "aucune", "keine", "nenhuma"},
	"uix.more":        {"%d more", "%d más", "%d de plus", "%d weitere", "mais %d"},

	// The home screen.
	"uix.open_run":    {"open or run", "abrir o ejecutar", "ouvrir ou lancer", "öffnen oder starten", "abrir ou executar"},
	"uix.options":     {"options", "opciones", "options", "Optionen", "opções"},
	"uix.type_search": {"type to search everything", "escribe para buscar en todo", "tapez pour chercher partout", "tippen sucht überall", "escreve para procurar em tudo"},
	"uix.back":        {"back", "volver", "retour", "zurück", "voltar"},
	"uix.run":         {"run", "ejecutar", "lancer", "starten", "executar"},
	"uix.clear":       {"clear", "borrar", "effacer", "löschen", "limpar"},
	"uix.type_value":  {"type the value", "escribe el valor", "tapez la valeur", "Wert eintippen", "escreve o valor"},

	// The guided run.
	"uix.tick":        {"tick or untick", "marcar o desmarcar", "cocher ou décocher", "an/aus", "marcar ou desmarcar"},
	"uix.start":       {"start", "empezar", "démarrer", "starten", "começar"},
	"uix.leave":       {"leave", "salir", "quitter", "beenden", "sair"},
	"uix.back_run":    {"back to the run", "volver al recorrido", "retour au parcours", "zurück zum Durchlauf", "voltar ao percurso"},
	"uix.go_on":       {"go on", "seguir", "continuer", "weiter", "continuar"},
	"uix.repeat":      {"repeat the last one", "repetir la anterior", "répéter la précédente", "letzte wiederholen", "repetir a anterior"},
	"uix.skip_it":     {"skip it", "saltarla", "la sauter", "überspringen", "saltar"},
	"uix.skip_next":   {"skip the next", "saltar la siguiente", "sauter la suivante", "nächste überspringen", "saltar a seguinte"},
	"uix.change_plan": {"change the plan", "cambiar el plan", "changer le plan", "Plan ändern", "mudar o plano"},
	"uix.finish":      {"finish here", "terminar aquí", "finir ici", "hier beenden", "terminar aqui"},
	"uix.stop_stage":  {"stop this stage", "parar esta etapa", "arrêter cette étape", "diese Etappe stoppen", "parar esta etapa"},
	"uix.end_stage":   {"end this stage, go to the next", "terminar esta etapa, pasar a la siguiente", "terminer cette étape, passer à la suivante", "Etappe beenden, zur nächsten", "terminar esta etapa, passar à seguinte"},

	// The unattended run.
	"uix.pilot_alone": {
		"nobody needs to answer anything: every stage runs on its own", "no hace falta responder nada: cada etapa se ejecuta sola",
		"personne n'a rien à répondre : chaque étape se lance seule", "niemand muss etwas beantworten: jede Etappe läuft von selbst",
		"ninguém precisa de responder a nada: cada etapa corre sozinha",
	},
	"uix.stop_run":   {"stop the run", "detener la ejecución", "arrêter l'exécution", "den Lauf anhalten", "parar a execução"},
	"uix.skip_stage": {"skip this stage", "saltar esta etapa", "sauter cette étape", "diese Etappe überspringen", "saltar esta etapa"},
	"uix.pilot_skipping": {
		"skipping this stage: the next one starts when it has ended", "saltando esta etapa: la siguiente empieza cuando termine",
		"étape sautée : la suivante commence dès qu'elle s'arrête", "Etappe wird übersprungen: die nächste beginnt, sobald sie endet",
		"a saltar esta etapa: a seguinte começa quando ela acabar",
	},
	"uix.pilot_stopping": {
		"stopping: the step that is running is ending", "parando: el paso en curso está terminando", "arrêt : l'étape en cours se termine",
		"wird gestoppt: der laufende Schritt endet gerade", "a parar: o passo em curso está a terminar",
	},
	"uix.leave_now": {"again to leave now", "otra vez para salir ya", "encore pour quitter tout de suite", "noch einmal für sofort beenden", "outra vez para sair já"},

	// The routine editor.
	"uix.add_remove": {"add/remove", "añadir/quitar", "ajouter/retirer", "hinzu/weg", "juntar/tirar"},
	"uix.reorder":    {"reorder", "ordenar", "ordonner", "ordnen", "ordenar"},
	"uix.empty":      {"clear", "vaciar", "vider", "leeren", "esvaziar"},
	"uix.undo":       {"undo", "deshacer", "annuler", "rückgängig", "desfazer"},
	"uix.save":       {"save", "guardar", "enregistrer", "speichern", "guardar"},
	"uix.ask_ai":     {"ask AI", "preguntar a la IA", "demander à l'IA", "KI fragen", "perguntar à IA"},
	"uix.unsaved": {
		"Unsaved changes. Save them?", "Hay cambios sin guardar. ¿Guardarlos?", "Modifications non enregistrées. Les enregistrer ?",
		"Ungespeicherte Änderungen. Speichern?", "Há alterações por guardar. Guardar?",
	},
	"uix.discard":      {"discard", "descartar", "abandonner", "verwerfen", "descartar"},
	"uix.keep_editing": {"any other key keeps editing", "otra tecla sigue editando", "autre touche : continuer", "andere Taste: weiter bearbeiten", "outra tecla continua a editar"},
	"uix.use_rec":      {"use it (you can still edit it)", "usarla (podrás editarla)", "l'utiliser (modifiable ensuite)", "übernehmen (bleibt bearbeitbar)", "usá-la (podes editá-la)"},
	"uix.keep_mine":    {"keep mine (the reasons stay as notes)", "quedarme con la mía (los motivos quedan como notas)", "garder la mienne (les raisons restent en notes)", "meine behalten (Gründe bleiben als Notizen)", "ficar com a minha (os motivos ficam como notas)"},
}

func init() {
	for k, v := range uiMessages {
		messages[k] = v
	}
}
