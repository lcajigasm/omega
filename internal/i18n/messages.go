package i18n

// messages maps each key to its translations in Supported order:
// en, es, fr, de, pt.
var messages = map[string][]string{
	// ---- root command ----
	"cli.short": {
		"AI-driven macOS maintenance: updates (topgrade) and cleans (Mole)",
		"Mantenimiento de macOS guiado por IA: actualiza (topgrade) y limpia (Mole)",
		"Maintenance de macOS pilotée par IA : met à jour (topgrade) et nettoie (Mole)",
		"KI-gesteuerte macOS-Wartung: aktualisiert (topgrade) und räumt auf (Mole)",
		"Manutenção do macOS guiada por IA: atualiza (topgrade) e limpa (Mole)",
	},
	"cli.long": {
		`omega uses an AI model (Anthropic, OpenAI, OpenRouter or OpenCode) to look after your Mac:
it updates all your software, cleans up what is left over, diagnoses errors and fixes them.

Tasks:
  full      complete maintenance (default)
  update    updates only (like topgrade)
  clean     cleanup only (like mo clean)
  analyze   report without changing anything
  fix       diagnose and fix the topgrade and Mole configuration

Examples:
  omega setup                        configure provider, API key and model
  omega                              complete maintenance
  omega update --yes                 update without asking
  omega -p "why does pipx fail?"     free-form task
  omega --provider openrouter --model openai/gpt-5.5 analyze`,
		`omega usa un modelo de IA (Anthropic, OpenAI, OpenRouter u OpenCode) para cuidar tu Mac:
actualiza todo el software, limpia lo que sobra, diagnostica los errores y los corrige.

Tareas:
  full      mantenimiento completo (por defecto)
  update    solo actualizar (como topgrade)
  clean     solo limpiar (como mo clean)
  analyze   informe sin cambiar nada
  fix       diagnosticar y arreglar la configuración de topgrade y Mole

Ejemplos:
  omega setup                        configurar proveedor, API key y modelo
  omega                              mantenimiento completo
  omega update --yes                 actualizar sin preguntar
  omega -p "¿por qué falla pipx?"    tarea libre
  omega --provider openrouter --model openai/gpt-5.5 analyze`,
		`omega utilise un modèle d'IA (Anthropic, OpenAI, OpenRouter ou OpenCode) pour entretenir votre Mac :
il met à jour tous vos logiciels, nettoie le superflu, diagnostique les erreurs et les corrige.

Tâches :
  full      maintenance complète (par défaut)
  update    mises à jour uniquement (comme topgrade)
  clean     nettoyage uniquement (comme mo clean)
  analyze   rapport sans rien modifier
  fix       diagnostiquer et corriger la configuration de topgrade et Mole

Exemples :
  omega setup                        configurer fournisseur, clé API et modèle
  omega                              maintenance complète
  omega update --yes                 mettre à jour sans confirmation
  omega -p "pourquoi pipx échoue ?"  tâche libre
  omega --provider openrouter --model openai/gpt-5.5 analyze`,
		`omega nutzt ein KI-Modell (Anthropic, OpenAI, OpenRouter oder OpenCode), um deinen Mac zu pflegen:
Es aktualisiert deine gesamte Software, räumt Überflüssiges auf, diagnostiziert Fehler und behebt sie.

Aufgaben:
  full      vollständige Wartung (Standard)
  update    nur aktualisieren (wie topgrade)
  clean     nur aufräumen (wie mo clean)
  analyze   Bericht, ohne etwas zu ändern
  fix       Konfiguration von topgrade und Mole diagnostizieren und reparieren

Beispiele:
  omega setup                        Anbieter, API-Key und Modell einrichten
  omega                              vollständige Wartung
  omega update --yes                 ohne Rückfrage aktualisieren
  omega -p "warum schlägt pipx fehl?"  freie Aufgabe
  omega --provider openrouter --model openai/gpt-5.5 analyze`,
		`omega usa um modelo de IA (Anthropic, OpenAI, OpenRouter ou OpenCode) para cuidar do seu Mac:
atualiza todo o software, limpa o que sobra, diagnostica os erros e corrige-os.

Tarefas:
  full      manutenção completa (padrão)
  update    apenas atualizar (como o topgrade)
  clean     apenas limpar (como o mo clean)
  analyze   relatório sem alterar nada
  fix       diagnosticar e corrigir a configuração do topgrade e do Mole

Exemplos:
  omega setup                        configurar fornecedor, API key e modelo
  omega                              manutenção completa
  omega update --yes                 atualizar sem perguntar
  omega -p "porque é que o pipx falha?"  tarefa livre
  omega --provider openrouter --model openai/gpt-5.5 analyze`,
	},
	"flag.provider": {
		"provider: anthropic, openai, openrouter, opencode",
		"proveedor: anthropic, openai, openrouter, opencode",
		"fournisseur : anthropic, openai, openrouter, opencode",
		"Anbieter: anthropic, openai, openrouter, opencode",
		"fornecedor: anthropic, openai, openrouter, opencode",
	},
	"flag.model": {
		"model to use (see `omega models`)",
		"modelo a usar (ver `omega models`)",
		"modèle à utiliser (voir `omega models`)",
		"zu verwendendes Modell (siehe `omega models`)",
		"modelo a usar (ver `omega models`)",
	},
	"flag.effort": {
		"effort for Claude models: low, medium, high, xhigh, max",
		"esfuerzo para modelos Claude: low, medium, high, xhigh, max",
		"effort pour les modèles Claude : low, medium, high, xhigh, max",
		"Aufwand für Claude-Modelle: low, medium, high, xhigh, max",
		"esforço para modelos Claude: low, medium, high, xhigh, max",
	},
	"flag.yes": {
		"auto-approve everything except blocked commands",
		"aprobar automáticamente todo excepto los comandos bloqueados",
		"tout approuver automatiquement sauf les commandes bloquées",
		"alles automatisch genehmigen außer blockierten Befehlen",
		"aprovar tudo automaticamente exceto comandos bloqueados",
	},
	"flag.no_tui": {
		"plain text output instead of the full-screen interface",
		"salida de texto simple en vez de la interfaz a pantalla completa",
		"sortie texte simple au lieu de l'interface plein écran",
		"einfache Textausgabe statt Vollbildoberfläche",
		"saída de texto simples em vez da interface em ecrã inteiro",
	},
	"flag.dry_run": {
		"read-only mode: the agent cannot change anything",
		"modo solo lectura: el agente no puede modificar nada",
		"mode lecture seule : l'agent ne peut rien modifier",
		"Nur-Lese-Modus: der Agent kann nichts ändern",
		"modo só de leitura: o agente não pode alterar nada",
	},
	"flag.sudo": {
		"ask for the sudo password up front and keep it alive during the run",
		"pedir la contraseña de sudo al inicio y mantenerla durante la ejecución",
		"demander le mot de passe sudo au départ et le garder actif pendant l'exécution",
		"sudo-Passwort zu Beginn abfragen und während des Laufs aktiv halten",
		"pedir a palavra-passe do sudo no início e mantê-la ativa durante a execução",
	},
	"flag.prompt": {
		"free-form task in natural language instead of a preset task",
		"tarea libre en lenguaje natural en lugar de una tarea predefinida",
		"tâche libre en langage naturel au lieu d'une tâche prédéfinie",
		"freie Aufgabe in natürlicher Sprache statt einer vordefinierten",
		"tarefa livre em linguagem natural em vez de uma tarefa predefinida",
	},
	"flag.once": {
		"exit when the agent finishes instead of asking for more instructions",
		"terminar cuando el agente acabe, sin pedir más instrucciones",
		"quitter quand l'agent a fini, sans demander d'autres instructions",
		"beenden, wenn der Agent fertig ist, statt nach weiteren Anweisungen zu fragen",
		"terminar quando o agente acabar, sem pedir mais instruções",
	},
	"err.unknown_task": {
		"unknown task %q (full, update, clean, analyze, fix)",
		"tarea desconocida %q (full, update, clean, analyze, fix)",
		"tâche inconnue %q (full, update, clean, analyze, fix)",
		"unbekannte Aufgabe %q (full, update, clean, analyze, fix)",
		"tarefa desconhecida %q (full, update, clean, analyze, fix)",
	},
	"err.unknown_provider": {
		"unknown provider %q (available: %s)",
		"proveedor desconocido %q (disponibles: %s)",
		"fournisseur inconnu %q (disponibles : %s)",
		"unbekannter Anbieter %q (verfügbar: %s)",
		"fornecedor desconhecido %q (disponíveis: %s)",
	},
	"err.no_key": {
		"no API key configured for %s (run `omega setup`, `omega auth set %s` or export %s)",
		"no hay API key configurada para %s (ejecuta `omega setup`, `omega auth set %s` o exporta %s)",
		"aucune clé API configurée pour %s (lancez `omega setup`, `omega auth set %s` ou exportez %s)",
		"kein API-Key für %s konfiguriert (führe `omega setup` oder `omega auth set %s` aus oder exportiere %s)",
		"nenhuma API key configurada para %s (execute `omega setup`, `omega auth set %s` ou exporte %s)",
	},
	"task.custom": {"custom", "personalizada", "personnalisée", "benutzerdefiniert", "personalizada"},
	"sudo.intro": {
		"omega needs sudo for some steps (e.g. macOS updates).",
		"omega necesita sudo para algunos pasos (p. ej. actualizaciones de macOS).",
		"omega a besoin de sudo pour certaines étapes (p. ex. mises à jour de macOS).",
		"omega benötigt sudo für einige Schritte (z. B. macOS-Updates).",
		"o omega precisa de sudo para alguns passos (p. ex. atualizações do macOS).",
	},
	"run.log": {"Run log: %s", "Registro de la ejecución: %s", "Journal d'exécution : %s", "Protokoll des Laufs: %s", "Registo da execução: %s"},

	// ---- setup ----
	"cmd.setup": {
		"Wizard: pick a provider, store the API key and choose a model",
		"Asistente: elegir proveedor, guardar la API key y escoger modelo",
		"Assistant : choisir un fournisseur, enregistrer la clé API et choisir un modèle",
		"Assistent: Anbieter wählen, API-Key speichern und Modell auswählen",
		"Assistente: escolher fornecedor, guardar a API key e escolher modelo",
	},
	"setup.providers":       {"Available providers:", "Proveedores disponibles:", "Fournisseurs disponibles :", "Verfügbare Anbieter:", "Fornecedores disponíveis:"},
	"setup.no_key":          {"no key", "sin key", "pas de clé", "kein Key", "sem key"},
	"setup.key_in":          {"key in %s", "key en %s", "clé dans %s", "Key in %s", "key em %s"},
	"setup.choose_provider": {"Provider (number or name)", "Proveedor (número o nombre)", "Fournisseur (numéro ou nom)", "Anbieter (Nummer oder Name)", "Fornecedor (número ou nome)"},
	"setup.replace_key": {
		"A key already exists (%s, %s). Replace it? y/N",
		"Ya hay una key (%s, %s). ¿Reemplazarla? s/N",
		"Une clé existe déjà (%s, %s). La remplacer ? o/N",
		"Es gibt bereits einen Key (%s, %s). Ersetzen? j/N",
		"Já existe uma key (%s, %s). Substituí-la? s/N",
	},
	"setup.get_key":   {"Get a key at %s", "Consigue una key en %s", "Obtenez une clé sur %s", "Einen Key erhältst du unter %s", "Obtenha uma key em %s"},
	"setup.enter_key": {"API key (input hidden)", "API key (no se mostrará)", "Clé API (saisie masquée)", "API-Key (Eingabe verborgen)", "API key (não será mostrada)"},
	"setup.no_key_entered": {
		"no key was entered", "no se ha introducido ninguna key", "aucune clé n'a été saisie", "es wurde kein Key eingegeben", "não foi introduzida nenhuma key",
	},
	"setup.key_saved": {
		"Key saved to the macOS Keychain.", "Key guardada en el llavero de macOS.", "Clé enregistrée dans le trousseau macOS.",
		"Key im macOS-Schlüsselbund gespeichert.", "Key guardada no porta-chaves do macOS.",
	},
	"setup.models_failed": {
		"Could not fetch the model list: %v", "No se pudo obtener la lista de modelos: %v", "Impossible de récupérer la liste des modèles : %v",
		"Modellliste konnte nicht abgerufen werden: %v", "Não foi possível obter a lista de modelos: %v",
	},
	"setup.done": {
		"\nAll set: %s · %s\nConfig at %s\nTry: omega analyze\n",
		"\nListo: %s · %s\nConfiguración en %s\nPrueba con: omega analyze\n",
		"\nC'est prêt : %s · %s\nConfiguration dans %s\nEssayez : omega analyze\n",
		"\nFertig: %s · %s\nKonfiguration in %s\nProbiere: omega analyze\n",
		"\nPronto: %s · %s\nConfiguração em %s\nExperimente: omega analyze\n",
	},
	"setup.models_count": {
		"\n%d models available. Type part of a name to filter, a number to pick, or Enter for %q.\n",
		"\n%d modelos disponibles. Escribe parte del nombre para filtrar, un número para elegir o Enter para %q.\n",
		"\n%d modèles disponibles. Tapez une partie du nom pour filtrer, un numéro pour choisir ou Entrée pour %q.\n",
		"\n%d Modelle verfügbar. Tippe einen Namensteil zum Filtern, eine Nummer zur Auswahl oder Enter für %q.\n",
		"\n%d modelos disponíveis. Escreva parte do nome para filtrar, um número para escolher ou Enter para %q.\n",
	},
	"setup.more": {
		"  … and %d more (filter to narrow down)\n", "  … y %d más (filtra para acotar)\n", "  … et %d de plus (filtrez pour affiner)\n",
		"  … und %d weitere (filtern zum Eingrenzen)\n", "  … e mais %d (filtre para restringir)\n",
	},
	"setup.model": {"Model", "Modelo", "Modèle", "Modell", "Modelo"},
	"setup.no_match": {
		"No model matches; using it as typed: %s", "Ningún modelo coincide; se usará tal cual: %s", "Aucun modèle ne correspond ; utilisé tel quel : %s",
		"Kein Modell passt; wird wie eingegeben verwendet: %s", "Nenhum modelo corresponde; será usado tal como escrito: %s",
	},

	// ---- auth ----
	"cmd.auth": {
		"Manage API keys (macOS Keychain)", "Gestionar las API keys (llavero de macOS)", "Gérer les clés API (trousseau macOS)",
		"API-Keys verwalten (macOS-Schlüsselbund)", "Gerir as API keys (porta-chaves do macOS)",
	},
	"cmd.auth_set": {
		"Store a provider's API key", "Guardar la API key de un proveedor", "Enregistrer la clé API d'un fournisseur",
		"API-Key eines Anbieters speichern", "Guardar a API key de um fornecedor",
	},
	"cmd.auth_remove": {
		"Delete a provider's API key from the Keychain", "Borrar del llavero la API key de un proveedor", "Supprimer la clé API d'un fournisseur du trousseau",
		"API-Key eines Anbieters aus dem Schlüsselbund löschen", "Apagar do porta-chaves a API key de um fornecedor",
	},
	"cmd.auth_status": {
		"Show which providers have a key", "Ver qué proveedores tienen key", "Voir quels fournisseurs ont une clé",
		"Anzeigen, welche Anbieter einen Key haben", "Ver que fornecedores têm key",
	},
	"auth.prompt": {"API key for %s", "API key de %s", "Clé API pour %s", "API-Key für %s", "API key de %s"},
	"auth.saved":  {"Saved.", "Guardada.", "Enregistrée.", "Gespeichert.", "Guardada."},

	// ---- models ----
	"cmd.models": {"List a provider's models", "Listar los modelos de un proveedor", "Lister les modèles d'un fournisseur", "Modelle eines Anbieters auflisten", "Listar os modelos de um fornecedor"},

	// ---- config ----
	"cmd.config": {
		"Show or change the configuration", "Ver o cambiar la configuración", "Afficher ou modifier la configuration",
		"Konfiguration anzeigen oder ändern", "Ver ou alterar a configuração",
	},
	"cmd.config_show": {"Show the current configuration", "Mostrar la configuración actual", "Afficher la configuration actuelle", "Aktuelle Konfiguration anzeigen", "Mostrar a configuração atual"},
	"cmd.config_path": {"Print the config file path", "Ruta del archivo de configuración", "Chemin du fichier de configuration", "Pfad der Konfigurationsdatei", "Caminho do ficheiro de configuração"},
	"cmd.config_set": {
		"Change a value: provider, model, effort, approval, language, command_timeout, skip_steps",
		"Cambiar un valor: provider, model, effort, approval, language, command_timeout, skip_steps",
		"Modifier une valeur : provider, model, effort, approval, language, command_timeout, skip_steps",
		"Einen Wert ändern: provider, model, effort, approval, language, command_timeout, skip_steps",
		"Alterar um valor: provider, model, effort, approval, language, command_timeout, skip_steps",
	},
	"cmd.config_edit": {"Open the configuration in $EDITOR", "Abrir la configuración en $EDITOR", "Ouvrir la configuration dans $EDITOR", "Konfiguration in $EDITOR öffnen", "Abrir a configuração no $EDITOR"},
	"config.bad_approval": {
		"approval must be dangerous, all or never", "approval debe ser dangerous, all o never", "approval doit valoir dangerous, all ou never",
		"approval muss dangerous, all oder never sein", "approval deve ser dangerous, all ou never",
	},
	"config.bad_duration": {
		"invalid duration (e.g. 45m): %v", "duración inválida (ej. 45m): %v", "durée invalide (ex. 45m) : %v", "ungültige Dauer (z. B. 45m): %v", "duração inválida (ex. 45m): %v",
	},
	"config.unknown_key": {"unknown key %q", "clave desconocida %q", "clé inconnue %q", "unbekannter Schlüssel %q", "chave desconhecida %q"},
	"config.bad_language": {
		"unsupported language %q (en, es, fr, de, pt or auto)", "idioma no soportado %q (en, es, fr, de, pt o auto)",
		"langue non prise en charge %q (en, es, fr, de, pt ou auto)", "nicht unterstützte Sprache %q (en, es, fr, de, pt oder auto)",
		"idioma não suportado %q (en, es, fr, de, pt ou auto)",
	},
	"config.read_error": {"reading %s: %v", "leyendo %s: %v", "lecture de %s : %v", "Lesen von %s: %v", "a ler %s: %v"},

	// ---- steps ----
	"cmd.steps": {
		"List the maintenance steps detected on this Mac", "Listar los pasos de mantenimiento detectados", "Lister les étapes de maintenance détectées",
		"Erkannte Wartungsschritte auflisten", "Listar os passos de manutenção detetados",
	},
	"cmd.steps_run":  {"Run steps directly, without AI", "Ejecutar pasos directamente, sin IA", "Exécuter des étapes directement, sans IA", "Schritte direkt ausführen, ohne KI", "Executar passos diretamente, sem IA"},
	"flag.steps_yes": {"don't ask for confirmation", "no pedir confirmación", "ne pas demander de confirmation", "nicht nach Bestätigung fragen", "não pedir confirmação"},
	"steps.legend": {
		"\n✓ available   – disabled   (blank) not installed", "\n✓ disponible   – desactivado   (vacío) no instalado",
		"\n✓ disponible   – désactivé   (vide) non installé", "\n✓ verfügbar   – deaktiviert   (leer) nicht installiert",
		"\n✓ disponível   – desativado   (vazio) não instalado",
	},
	"steps.unknown": {"no step %q (see omega steps)", "no existe el paso %q (omega steps)", "aucune étape %q (voir omega steps)", "kein Schritt %q (siehe omega steps)", "não existe o passo %q (omega steps)"},
	"steps.not_installed": {
		"· %s is not installed, skipping", "· %s no está instalado, se omite", "· %s n'est pas installé, ignoré", "· %s ist nicht installiert, übersprungen", "· %s não está instalado, ignorado",
	},
	"steps.confirm": {"%s → `%s`. Run it? y/N", "%s → `%s`. ¿Ejecutar? s/N", "%s → `%s`. L'exécuter ? o/N", "%s → `%s`. Ausführen? j/N", "%s → `%s`. Executar? s/N"},
	"steps.failed":  {"✗ %s failed (exit %d)", "✗ %s falló (exit %d)", "✗ %s a échoué (exit %d)", "✗ %s fehlgeschlagen (exit %d)", "✗ %s falhou (exit %d)"},
	"steps.failed_summary": {
		"%d step(s) failed; try `omega fix` or `omega -p \"...\"` to let the AI diagnose it",
		"%d paso(s) fallaron; prueba `omega fix` o `omega -p \"...\"` para que la IA lo diagnostique",
		"%d étape(s) en échec ; essayez `omega fix` ou `omega -p \"...\"` pour que l'IA diagnostique",
		"%d Schritt(e) fehlgeschlagen; versuche `omega fix` oder `omega -p \"...\"`, damit die KI es diagnostiziert",
		"%d passo(s) falharam; experimente `omega fix` ou `omega -p \"...\"` para a IA diagnosticar",
	},
	"cat.check":   {"CHECKS", "COMPROBACIONES", "VÉRIFICATIONS", "PRÜFUNGEN", "VERIFICAÇÕES"},
	"cat.update":  {"UPDATES", "ACTUALIZACIONES", "MISES À JOUR", "UPDATES", "ATUALIZAÇÕES"},
	"cat.clean":   {"CLEANUP", "LIMPIEZA", "NETTOYAGE", "AUFRÄUMEN", "LIMPEZA"},
	"cat.backend": {"BACKENDS (topgrade / Mole)", "BACKENDS (topgrade / Mole)", "BACKENDS (topgrade / Mole)", "BACKENDS (topgrade / Mole)", "BACKENDS (topgrade / Mole)"},

	// ---- notes ----
	"cmd.notes": {
		"Show what omega has learned about this Mac", "Ver lo que omega ha aprendido de esta máquina", "Voir ce qu'omega a appris sur ce Mac",
		"Anzeigen, was omega über diesen Mac gelernt hat", "Ver o que o omega aprendeu sobre este Mac",
	},
	"cmd.notes_clear": {"Delete the notes", "Borrar las notas", "Supprimer les notes", "Notizen löschen", "Apagar as notas"},
	"notes.empty":     {"No notes yet.", "Aún no hay notas.", "Pas encore de notes.", "Noch keine Notizen.", "Ainda não há notas."},

	// ---- interface ----
	"ui.starting":     {"starting…", "iniciando…", "démarrage…", "starte…", "a iniciar…"},
	"ui.task":         {"task: %s", "tarea: %s", "tâche : %s", "Aufgabe: %s", "tarefa: %s"},
	"ui.read_only":    {"read-only", "solo lectura", "lecture seule", "nur lesen", "só leitura"},
	"ui.thinking":     {"thinking…  (ctrl+c cancels)", "pensando…  (ctrl+c cancela)", "réflexion…  (ctrl+c annule)", "denke nach…  (Strg+C bricht ab)", "a pensar…  (ctrl+c cancela)"},
	"ui.hint_running": {"ctrl+c cancels · ↑/↓ scroll", "ctrl+c cancela · ↑/↓ desplazar", "ctrl+c annule · ↑/↓ défiler", "Strg+C bricht ab · ↑/↓ scrollen", "ctrl+c cancela · ↑/↓ deslocar"},
	"ui.hint_done":    {"q quit · ↑/↓ scroll", "q salir · ↑/↓ desplazar", "q quitter · ↑/↓ défiler", "q beenden · ↑/↓ scrollen", "q sair · ↑/↓ deslocar"},
	"ui.press_q":      {"Press q to quit.", "Pulsa q para salir.", "Appuyez sur q pour quitter.", "Drücke q zum Beenden.", "Prima q para sair."},
	"ui.placeholder": {
		"Ask for something else, or press Enter on empty / Esc to quit",
		"Pide algo más, o pulsa Enter vacío / Esc para salir",
		"Demandez autre chose, ou Entrée à vide / Échap pour quitter",
		"Frag nach etwas anderem, oder leeres Enter / Esc zum Beenden",
		"Peça mais alguma coisa, ou prima Enter vazio / Esc para sair",
	},
	"ui.followup": {
		"› (empty Enter to quit) ", "› (Enter vacío para salir) ", "› (Entrée à vide pour quitter) ", "› (leeres Enter zum Beenden) ", "› (Enter vazio para sair) ",
	},
	"ui.summary": {
		"Done. Tokens: %d in / %d out. Log: %s", "Fin. Tokens: %d entrada / %d salida. Registro: %s",
		"Terminé. Jetons : %d entrée / %d sortie. Journal : %s", "Fertig. Tokens: %d Eingabe / %d Ausgabe. Protokoll: %s",
		"Concluído. Tokens: %d entrada / %d saída. Registo: %s",
	},
	"ui.confirm":      {"Run this?", "¿Ejecutar?", "Exécuter ?", "Ausführen?", "Executar?"},
	"ui.risk":         {"risk: %s", "riesgo: %s", "risque : %s", "Risiko: %s", "risco: %s"},
	"ui.confirm_keys": {"[y] yes   [a] yes to all   [n] no", "[y] sí   [a] sí a todo   [n] no", "[y] oui   [a] oui à tout   [n] non", "[y] ja   [a] ja zu allem   [n] nein", "[y] sim   [a] sim a tudo   [n] não"},
	"ui.confirm_line": {"  [y] yes  [a] yes to all  [n] no: ", "  [y] sí  [a] sí a todo  [n] no: ", "  [y] oui  [a] oui à tout  [n] non : ", "  [y] ja  [a] ja zu allem  [n] nein: ", "  [y] sim  [a] sim a tudo  [n] não: "},
	"ui.approved":     {"approved", "aprobado", "approuvé", "genehmigt", "aprovado"},
	"ui.approved_all": {"approved (all)", "aprobado (todo)", "approuvé (tout)", "genehmigt (alle)", "aprovado (tudo)"},
	"ui.denied":       {"denied", "rechazado", "refusé", "abgelehnt", "recusado"},
	"ui.no_tty": {
		"  no terminal: denied (use --yes to approve automatically)", "  sin terminal: rechazado (usa --yes para aprobar automáticamente)",
		"  pas de terminal : refusé (utilisez --yes pour approuver automatiquement)", "  kein Terminal: abgelehnt (mit --yes automatisch genehmigen)",
		"  sem terminal: recusado (use --yes para aprovar automaticamente)",
	},
	"risk.safe":      {"read-only", "lectura", "lecture", "lesend", "leitura"},
	"risk.normal":    {"normal", "normal", "normal", "normal", "normal"},
	"risk.dangerous": {"dangerous", "peligroso", "dangereux", "gefährlich", "perigoso"},
	"risk.blocked":   {"blocked", "bloqueado", "bloqué", "blockiert", "bloqueado"},

	// ---- agent and providers ----
	"agent.cancelled":   {"Cancelled.", "Cancelado.", "Annulé.", "Abgebrochen.", "Cancelado."},
	"agent.turn_limit":  {"turn limit of %d reached", "límite de %d turnos alcanzado", "limite de %d tours atteinte", "Limit von %d Runden erreicht", "limite de %d turnos atingido"},
	"agent.model_error": {"Model error: %s", "Error del modelo: %s", "Erreur du modèle : %s", "Modellfehler: %s", "Erro do modelo: %s"},
	"agent.note_saved":  {"note saved: %s", "nota guardada: %s", "note enregistrée : %s", "Notiz gespeichert: %s", "nota guardada: %s"},
	"agent.read":        {"read %s", "leer %s", "lire %s", "lesen %s", "ler %s"},
	"agent.write":       {"write %s (%d → %d bytes)", "escribir %s (%d → %d bytes)", "écrire %s (%d → %d octets)", "schreiben %s (%d → %d Bytes)", "escrever %s (%d → %d bytes)"},
	"agent.disk":        {"measure disk usage in %s", "medir espacio en %s", "mesurer l'espace dans %s", "Speicherbelegung in %s messen", "medir espaço em %s"},
	"llm.invalid_key":   {"invalid API key", "API key inválida", "clé API invalide", "ungültiger API-Key", "API key inválida"},
	"llm.not_found":     {"model or endpoint not found", "modelo o endpoint no encontrado", "modèle ou endpoint introuvable", "Modell oder Endpunkt nicht gefunden", "modelo ou endpoint não encontrado"},
	"llm.rate_limit":    {"rate limit reached", "límite de peticiones alcanzado", "limite de requêtes atteinte", "Ratenlimit erreicht", "limite de pedidos atingido"},
	"secrets.save":      {"saving to the Keychain: %v: %s", "guardando en el llavero: %v: %s", "enregistrement dans le trousseau : %v : %s", "Speichern im Schlüsselbund: %v: %s", "a guardar no porta-chaves: %v: %s"},
	"secrets.delete":    {"deleting from the Keychain: %v: %s", "borrando del llavero: %v: %s", "suppression du trousseau : %v : %s", "Löschen aus dem Schlüsselbund: %v: %s", "a apagar do porta-chaves: %v: %s"},

	// ---- step names (ids without an entry keep their English name) ----
	"step.disk":                  {"Disk space", "Espacio en disco", "Espace disque", "Speicherplatz", "Espaço em disco"},
	"step.brew-outdated":         {"Outdated Homebrew packages", "Paquetes de Homebrew desactualizados", "Paquets Homebrew obsolètes", "Veraltete Homebrew-Pakete", "Pacotes do Homebrew desatualizados"},
	"step.macos-check":           {"macOS updates", "Actualizaciones de macOS", "Mises à jour de macOS", "macOS-Updates", "Atualizações do macOS"},
	"step.brew-greedy":           {"Self-updating casks", "Casks con actualización propia", "Casks à mise à jour automatique", "Selbstaktualisierende Casks", "Casks com atualização própria"},
	"step.npm":                   {"npm global packages", "Paquetes globales de npm", "Paquets npm globaux", "Globale npm-Pakete", "Pacotes globais do npm"},
	"step.pnpm":                  {"pnpm global packages", "Paquetes globales de pnpm", "Paquets pnpm globaux", "Globale pnpm-Pakete", "Pacotes globais do pnpm"},
	"step.cargo":                 {"cargo binaries", "Binarios de cargo", "Binaires cargo", "cargo-Programme", "Binários do cargo"},
	"step.gh-ext":                {"gh extensions", "Extensiones de gh", "Extensions gh", "gh-Erweiterungen", "Extensões do gh"},
	"step.brew-cleanup":          {"Homebrew cleanup", "Limpieza de Homebrew", "Nettoyage de Homebrew", "Homebrew aufräumen", "Limpeza do Homebrew"},
	"step.brew-autoremove":       {"Orphaned Homebrew dependencies", "Dependencias huérfanas de Homebrew", "Dépendances Homebrew orphelines", "Verwaiste Homebrew-Abhängigkeiten", "Dependências órfãs do Homebrew"},
	"step.npm-cache":             {"npm cache", "Caché de npm", "Cache npm", "npm-Cache", "Cache do npm"},
	"step.pnpm-store":            {"pnpm store", "Almacén de pnpm", "Store pnpm", "pnpm-Store", "Armazém do pnpm"},
	"step.pip-cache":             {"pip cache", "Caché de pip", "Cache pip", "pip-Cache", "Cache do pip"},
	"step.uv-cache":              {"uv cache", "Caché de uv", "Cache uv", "uv-Cache", "Cache do uv"},
	"step.go-cache":              {"Go build cache", "Caché de compilación de Go", "Cache de compilation Go", "Go-Build-Cache", "Cache de compilação do Go"},
	"step.simulators":            {"Unavailable simulators", "Simuladores no disponibles", "Simulateurs indisponibles", "Nicht verfügbare Simulatoren", "Simuladores indisponíveis"},
	"step.docker":                {"Docker (unused images and stopped containers)", "Docker (imágenes sin usar y contenedores parados)", "Docker (images inutilisées et conteneurs arrêtés)", "Docker (ungenutzte Images und gestoppte Container)", "Docker (imagens não usadas e contentores parados)"},
	"step.trash":                 {"Trash", "Papelera", "Corbeille", "Papierkorb", "Lixo"},
	"step.topgrade-preview":      {"topgrade (dry run)", "topgrade (simulación)", "topgrade (simulation)", "topgrade (Probelauf)", "topgrade (simulação)"},
	"step.mole-clean-preview":    {"Mole clean (dry run)", "Mole clean (simulación)", "Mole clean (simulation)", "Mole clean (Probelauf)", "Mole clean (simulação)"},
	"step.mole-optimize-preview": {"Mole optimize (dry run)", "Mole optimize (simulación)", "Mole optimize (simulation)", "Mole optimize (Probelauf)", "Mole optimize (simulação)"},
	"step.mole-update":           {"Update Mole", "Actualizar Mole", "Mettre à jour Mole", "Mole aktualisieren", "Atualizar o Mole"},

	// ---- key validation ----
	"setup.key_checking": {"Checking the key with %s…", "Comprobando la key con %s…", "Vérification de la clé auprès de %s…", "Prüfe den Key bei %s…", "A verificar a key com %s…"},
	"setup.key_ok":       {"Key accepted.", "Key aceptada.", "Clé acceptée.", "Key akzeptiert.", "Key aceite."},
	"setup.key_url": {
		"That does not look like an API key (it looks like a URL or text with spaces). Copy the key itself from %s",
		"Eso no parece una API key (parece una URL o un texto con espacios). Copia la key en sí desde %s",
		"Cela ne ressemble pas à une clé API (on dirait une URL ou un texte avec des espaces). Copiez la clé elle même depuis %s",
		"Das sieht nicht nach einem API-Key aus (eher nach einer URL oder Text mit Leerzeichen). Kopiere den Key selbst von %s",
		"Isso não parece uma API key (parece um URL ou texto com espaços). Copie a própria key em %s",
	},
	"setup.key_prefix": {
		"Warning: %s keys usually start with %q.", "Aviso: las keys de %s suelen empezar por %q.", "Attention : les clés %s commencent généralement par %q.",
		"Hinweis: %s-Keys beginnen normalerweise mit %q.", "Aviso: as keys de %s costumam começar por %q.",
	},
	"setup.key_rejected": {
		"%s rejected this key, so it was not saved: %v", "%s ha rechazado esta key, así que no se ha guardado: %v", "%s a refusé cette clé, elle n'a donc pas été enregistrée : %v",
		"%s hat diesen Key abgelehnt, er wurde nicht gespeichert: %v", "%s rejeitou esta key, por isso não foi guardada: %v",
	},
	"setup.key_unverified": {
		"Could not verify the key (%v); saving it anyway.", "No se pudo verificar la key (%v); se guarda igualmente.", "Impossible de vérifier la clé (%v) ; elle est enregistrée quand même.",
		"Key konnte nicht geprüft werden (%v); wird trotzdem gespeichert.", "Não foi possível verificar a key (%v); é guardada na mesma.",
	},
	"auth.looks_wrong": {
		"this is not an API key, run `omega auth set %s`", "esto no es una API key, ejecuta `omega auth set %s`", "ce n'est pas une clé API, lancez `omega auth set %s`",
		"das ist kein API-Key, führe `omega auth set %s` aus", "isto não é uma API key, execute `omega auth set %s`",
	},
}
