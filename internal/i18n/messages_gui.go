package i18n

import "strings"

// Messages of the desktop app. Same order as messages: en, es, fr, de, pt.
var guiMessages = map[string][]string{
	"gui.task.full":           {"Full maintenance", "Mantenimiento completo", "Maintenance complète", "Komplette Wartung", "Manutenção completa"},
	"gui.task.full.detail":    {"Update everything, then clean up", "Actualiza todo y después limpia", "Tout mettre à jour, puis nettoyer", "Alles aktualisieren, dann aufräumen", "Atualiza tudo e depois limpa"},
	"gui.task.update":         {"Update", "Actualizar", "Mettre à jour", "Aktualisieren", "Atualizar"},
	"gui.task.update.detail":  {"Bring installed software up to date", "Pone al día el software instalado", "Mettre à jour les logiciels installés", "Installierte Software auf den neuesten Stand bringen", "Põe em dia o software instalado"},
	"gui.task.clean":          {"Clean", "Limpiar", "Nettoyer", "Aufräumen", "Limpar"},
	"gui.task.clean.detail":   {"Free disk space, measuring first", "Libera espacio en disco, midiendo antes", "Libérer de l'espace disque, en mesurant d'abord", "Speicherplatz freigeben, erst messen", "Liberta espaço em disco, medindo primeiro"},
	"gui.task.analyze":        {"Analyze", "Analizar", "Analyser", "Analysieren", "Analisar"},
	"gui.task.analyze.detail": {"Report on the machine, changes nothing", "Informe del equipo, no cambia nada", "Rapport sur la machine, ne change rien", "Bericht über den Rechner, ändert nichts", "Relatório da máquina, não altera nada"},
	"gui.task.fix":            {"Fix", "Reparar", "Réparer", "Reparieren", "Reparar"},
	"gui.task.fix.detail":     {"Find what is broken and repair it", "Busca lo que está roto y lo repara", "Trouver ce qui est cassé et le réparer", "Finden, was kaputt ist, und reparieren", "Encontra o que está avariado e repara"},
	"gui.task.tune":           {"Tune", "Optimizar", "Optimiser", "Optimieren", "Otimizar"},
	"gui.task.tune.detail":    {"Review the configuration of your tools", "Revisa la configuración de tus herramientas", "Revoir la configuration de vos outils", "Die Konfiguration deiner Tools prüfen", "Revê a configuração das suas ferramentas"},

	"gui.ask":                {"Or say what you need…", "O di qué necesitas…", "Ou dites ce qu'il vous faut…", "Oder sag, was du brauchst…", "Ou diga o que precisa…"},
	"gui.reply":              {"Reply to omega…", "Responde a omega…", "Répondre à omega…", "omega antworten…", "Responder ao omega…"},
	"gui.send":               {"Send", "Enviar", "Envoyer", "Senden", "Enviar"},
	"gui.dry_run":            {"Read only: offer changes, make none", "Solo lectura: propone cambios, no hace ninguno", "Lecture seule : proposer, ne rien changer", "Nur lesen: Änderungen vorschlagen, keine machen", "Só leitura: propõe alterações, não faz nenhuma"},
	"gui.stop":               {"Stop", "Detener", "Arrêter", "Stoppen", "Parar"},
	"gui.new":                {"New session", "Nueva sesión", "Nouvelle session", "Neue Sitzung", "Nova sessão"},
	"gui.working":            {"Working…", "Trabajando…", "En cours…", "Arbeite…", "A trabalhar…"},
	"gui.ended":              {"Session ended", "Sesión terminada", "Session terminée", "Sitzung beendet", "Sessão terminada"},
	"gui.approve":            {"Run", "Ejecutar", "Exécuter", "Ausführen", "Executar"},
	"gui.approve_all":        {"Run, and stop asking", "Ejecutar y no preguntar más", "Exécuter et ne plus demander", "Ausführen, nicht mehr fragen", "Executar e não perguntar mais"},
	"gui.deny":               {"Don't run", "No ejecutar", "Ne pas exécuter", "Nicht ausführen", "Não executar"},
	"gui.choose":             {"Do the selected", "Hacer lo seleccionado", "Faire la sélection", "Auswahl ausführen", "Fazer o selecionado"},
	"gui.choose_none":        {"None of these", "Ninguna", "Aucune", "Nichts davon", "Nenhuma"},
	"gui.password":           {"Password", "Contraseña", "Mot de passe", "Passwort", "Palavra-passe"},
	"gui.password_ok":        {"Continue", "Continuar", "Continuer", "Weiter", "Continuar"},
	"gui.password_no":        {"Skip what needs admin access", "Saltar lo que requiere permisos", "Ignorer ce qui demande des droits", "Admin-Schritte überspringen", "Saltar o que precisa de permissões"},
	"gui.tokens":             {"tokens", "tokens", "jetons", "Tokens", "tokens"},
	"gui.lines":              {"lines", "líneas", "lignes", "Zeilen", "linhas"},
	"gui.risk.normal":        {"normal", "normal", "normal", "normal", "normal"},
	"gui.risk.safe":          {"read-only", "solo lectura", "lecture seule", "nur lesen", "só leitura"},
	"gui.risk.danger":        {"dangerous", "peligroso", "dangereux", "gefährlich", "perigoso"},
	"gui.demo":               {"Demo: nothing is run and no API is called", "Demo: no se ejecuta nada ni se llama a ninguna API", "Démo : rien n'est exécuté, aucune API appelée", "Demo: nichts wird ausgeführt, keine API aufgerufen", "Demo: nada é executado nem é chamada nenhuma API"},
	"gui.start_failed":       {"The session could not start", "No se pudo iniciar la sesión", "La session n'a pas pu démarrer", "Die Sitzung konnte nicht starten", "Não foi possível iniciar a sessão"},
	"gui.setup":              {"Provider and key", "Proveedor y clave", "Fournisseur et clé", "Anbieter und Schlüssel", "Fornecedor e chave"},
	"gui.setup_open":         {"Set up", "Configurar", "Configurer", "Einrichten", "Configurar"},
	"gui.setup_intro":        {"Choose who runs the model. The key is checked with the provider and stored in this system's keychain.", "Elige quién ejecuta el modelo. La clave se comprueba con el proveedor y se guarda en el llavero del sistema.", "Choisissez qui exécute le modèle. La clé est vérifiée auprès du fournisseur et rangée dans le trousseau du système.", "Wähle, wer das Modell ausführt. Der Schlüssel wird beim Anbieter geprüft und im Schlüsselbund des Systems abgelegt.", "Escolha quem executa o modelo. A chave é verificada com o fornecedor e guardada no porta-chaves do sistema."},
	"gui.key":                {"API key", "Clave de API", "Clé d'API", "API-Schlüssel", "Chave de API"},
	"gui.key_keep":           {"Leave empty to keep the saved key", "Déjalo vacío para conservar la clave guardada", "Laisser vide pour garder la clé enregistrée", "Leer lassen, um den gespeicherten Schlüssel zu behalten", "Deixe vazio para manter a chave guardada"},
	"gui.get_key":            {"Get a key", "Conseguir una clave", "Obtenir une clé", "Schlüssel besorgen", "Obter uma chave"},
	"gui.more_info":          {"How to install it", "Cómo instalarlo", "Comment l'installer", "So installierst du es", "Como instalar"},
	"gui.base_url":           {"Endpoint URL (OpenAI-compatible)", "URL del endpoint (compatible con OpenAI)", "URL du point d'accès (compatible OpenAI)", "Endpunkt-URL (OpenAI-kompatibel)", "URL do endpoint (compatível com OpenAI)"},
	"gui.base_url_req":       {"this provider needs an endpoint URL", "este proveedor necesita la URL del endpoint", "ce fournisseur a besoin de l'URL du point d'accès", "dieser Anbieter braucht eine Endpunkt-URL", "este fornecedor precisa do URL do endpoint"},
	"gui.save":               {"Save", "Guardar", "Enregistrer", "Speichern", "Guardar"},
	"gui.saving":             {"Checking…", "Comprobando…", "Vérification…", "Prüfe…", "A verificar…"},
	"gui.saved":              {"Saved: %s · %s", "Guardado: %s · %s", "Enregistré : %s · %s", "Gespeichert: %s · %s", "Guardado: %s · %s"},
	"gui.back":               {"Back", "Volver", "Retour", "Zurück", "Voltar"},
	"gui.in_use":             {"in use", "en uso", "utilisé", "in Verwendung", "em uso"},
	"gui.not_set":            {"not set up yet", "sin configurar", "pas encore configuré", "noch nicht eingerichtet", "ainda por configurar"},
	"gui.unknown_prov":       {"unknown provider: %s", "proveedor desconocido: %s", "fournisseur inconnu : %s", "unbekannter Anbieter: %s", "fornecedor desconhecido: %s"},
	"gui.task.health":        {"Health and security", "Salud y seguridad", "Santé et sécurité", "Zustand und Sicherheit", "Saúde e segurança"},
	"gui.task.health.detail": {"Disk, backups, firewall, encryption and logs", "Disco, copias, cortafuegos, cifrado y registros", "Disque, sauvegardes, pare-feu, chiffrement et journaux", "Festplatte, Backups, Firewall, Verschlüsselung und Protokolle", "Disco, cópias, firewall, cifragem e registos"},
	"gui.task.dev":           {"Developer tools", "Herramientas de desarrollo", "Outils de développement", "Entwicklerwerkzeuge", "Ferramentas de desenvolvimento"},
	"gui.task.dev.detail":    {"Languages, version managers and PATH", "Lenguajes, gestores de versiones y PATH", "Langages, gestionnaires de versions et PATH", "Sprachen, Versionsmanager und PATH", "Linguagens, gestores de versões e PATH"},
	"gui.task.mac":           {"What runs on your Mac", "Qué se ejecuta en tu Mac", "Ce qui tourne sur votre Mac", "Was auf deinem Mac läuft", "O que corre no seu Mac"},
	"gui.task.mac.detail":    {"Login items, background jobs, menu bar and Dock", "Arranque, segundo plano, barra de menús y Dock", "Ouverture, arrière-plan, barre des menus et Dock", "Anmeldeobjekte, Hintergrund, Menüleiste und Dock", "Arranque, segundo plano, barra de menus e Dock"},
	"gui.task.docker":        {"Docker space", "Espacio de Docker", "Espace de Docker", "Docker-Speicher", "Espaço do Docker"},
	"gui.task.docker.detail": {"See what it uses and clean only what you pick", "Mira lo que ocupa y limpia solo lo que elijas", "Voir ce qu'il occupe et ne nettoyer que votre choix", "Sehen, was es belegt, und nur Gewähltes löschen", "Vê o que ocupa e limpa só o que escolher"},

	"gui.nav.home":     {"Maintenance", "Mantenimiento", "Maintenance", "Wartung", "Manutenção"},
	"gui.nav.session":  {"Session", "Sesión", "Session", "Sitzung", "Sessão"},
	"gui.nav.history":  {"History", "Historial", "Historique", "Verlauf", "Histórico"},
	"gui.nav.system":   {"Status", "Estado", "État", "Status", "Estado"},
	"gui.nav.settings": {"Settings", "Ajustes", "Réglages", "Einstellungen", "Definições"},

	"gui.home.title":      {"What does this computer need?", "¿Qué necesita este equipo?", "De quoi cet ordinateur a-t-il besoin ?", "Was braucht dieser Rechner?", "De que precisa este computador?"},
	"gui.home.start":      {"Start", "Empezar", "Démarrer", "Starten", "Começar"},
	"gui.home.more":       {"One thing at a time", "Una cosa cada vez", "Une chose à la fois", "Eins nach dem anderen", "Uma coisa de cada vez"},
	"gui.home.ask_title":  {"Or ask in your own words", "O pídelo con tus palabras", "Ou demandez avec vos mots", "Oder frag mit eigenen Worten", "Ou peça por palavras suas"},
	"gui.home.last_run":   {"Last session", "Última sesión", "Dernière session", "Letzte Sitzung", "Última sessão"},
	"gui.home.never":      {"none yet", "ninguna todavía", "aucune pour l'instant", "noch keine", "ainda nenhuma"},
	"gui.home.provider":   {"Provider", "Proveedor", "Fournisseur", "Anbieter", "Fornecedor"},
	"gui.home.running":    {"A session is running. It has to end before another starts.", "Hay una sesión en marcha. Tiene que terminar antes de empezar otra.", "Une session est en cours. Elle doit se terminer avant d'en lancer une autre.", "Eine Sitzung läuft. Sie muss enden, bevor eine neue startet.", "Há uma sessão em curso. Tem de terminar antes de começar outra."},
	"gui.home.view":       {"View session", "Ver sesión", "Voir la session", "Sitzung ansehen", "Ver sessão"},
	"gui.home.need_setup": {"Choose a provider and add its key before starting.", "Elige un proveedor y añade su clave antes de empezar.", "Choisissez un fournisseur et ajoutez sa clé avant de commencer.", "Wähle einen Anbieter und hinterlege den Schlüssel, bevor du startest.", "Escolha um fornecedor e adicione a chave antes de começar."},

	"gui.session.none":    {"No session yet. Start one from Maintenance.", "Aún no hay sesión. Empieza una desde Mantenimiento.", "Pas encore de session. Lancez-en une depuis Maintenance.", "Noch keine Sitzung. Starte eine unter Wartung.", "Ainda não há sessão. Comece uma em Manutenção."},
	"gui.session.answer":  {"Waiting for your answer", "Esperando tu respuesta", "En attente de votre réponse", "Wartet auf deine Antwort", "À espera da sua resposta"},
	"gui.session.message": {"Done. Reply to continue, or start a new session.", "Hecho. Responde para seguir o empieza otra sesión.", "Terminé. Répondez pour continuer, ou lancez une nouvelle session.", "Fertig. Antworte, um fortzufahren, oder starte eine neue Sitzung.", "Feito. Responda para continuar ou comece outra sessão."},
	"gui.mode.ask":        {"Ask", "Preguntar", "Demander", "Fragen", "Perguntar"},
	"gui.mode.auto":       {"Automatic", "Automático", "Automatique", "Automatisch", "Automático"},
	"gui.mode.readonly":   {"Read only", "Solo lectura", "Lecture seule", "Nur lesen", "Só leitura"},

	"gui.history.intro": {"Every session is saved on this computer, and nowhere else.", "Cada sesión se guarda en este equipo y en ningún otro sitio.", "Chaque session est enregistrée sur cet ordinateur, et nulle part ailleurs.", "Jede Sitzung wird auf diesem Rechner gespeichert, nirgends sonst.", "Cada sessão fica guardada neste computador e em mais lado nenhum."},
	"gui.history.pick":  {"Choose a session to read it.", "Elige una sesión para leerla.", "Choisissez une session pour la lire.", "Wähle eine Sitzung, um sie zu lesen.", "Escolha uma sessão para a ler."},
	"gui.copy":          {"Copy", "Copiar", "Copier", "Kopieren", "Copiar"},
	"gui.copied":        {"Copied", "Copiado", "Copié", "Kopiert", "Copiado"},

	"gui.system.check":       {"Setup check", "Comprobación", "Vérification", "Prüfung", "Verificação"},
	"gui.system.check_intro": {"Version, configuration, provider and key.", "Versión, configuración, proveedor y clave.", "Version, configuration, fournisseur et clé.", "Version, Konfiguration, Anbieter und Schlüssel.", "Versão, configuração, fornecedor e chave."},
	"gui.system.run":         {"Check now", "Comprobar ahora", "Vérifier maintenant", "Jetzt prüfen", "Verificar agora"},
	"gui.system.ok":          {"Everything is in order", "Todo está en orden", "Tout est en ordre", "Alles in Ordnung", "Está tudo em ordem"},
	"gui.system.bad":         {"Something needs attention", "Algo necesita atención", "Quelque chose demande votre attention", "Etwas braucht Aufmerksamkeit", "Algo precisa de atenção"},
	"gui.system.steps":       {"Maintenance steps", "Pasos de mantenimiento", "Étapes de maintenance", "Wartungsschritte", "Passos de manutenção"},
	"gui.system.steps_intro": {"What omega can run on this computer. Turn off what it should never run.", "Lo que omega puede ejecutar en este equipo. Desactiva lo que no deba ejecutar nunca.", "Ce qu'omega peut exécuter sur cet ordinateur. Désactivez ce qu'il ne doit jamais exécuter.", "Was omega auf diesem Rechner ausführen kann. Schalte ab, was es nie ausführen soll.", "O que o omega pode executar neste computador. Desative o que nunca deve executar."},
	"gui.system.count":       {"%d of %d installed", "%d de %d instalados", "%d sur %d installées", "%d von %d installiert", "%d de %d instalados"},
	"gui.system.missing":     {"not installed", "no instalado", "non installé", "nicht installiert", "não instalado"},
	"gui.system.notes":       {"Notes", "Notas", "Notes", "Notizen", "Notas"},
	"gui.system.notes_intro": {"What omega has learned about this computer between sessions.", "Lo que omega ha aprendido de este equipo entre sesiones.", "Ce qu'omega a appris de cet ordinateur d'une session à l'autre.", "Was omega zwischen den Sitzungen über diesen Rechner gelernt hat.", "O que o omega aprendeu sobre este computador entre sessões."},
	"gui.system.notes_clear": {"Forget the notes", "Olvidar las notas", "Oublier les notes", "Notizen vergessen", "Esquecer as notas"},

	"gui.set.model":          {"Model", "Modelo", "Modèle", "Modell", "Modelo"},
	"gui.set.model_intro":    {"Automatic uses a fast model for routine work and a stronger one when something fails.", "Automático usa un modelo rápido para lo rutinario y uno más potente cuando algo falla.", "Automatique utilise un modèle rapide pour la routine et un plus puissant en cas d'échec.", "Automatisch nutzt ein schnelles Modell für Routine und ein stärkeres, wenn etwas fehlschlägt.", "Automático usa um modelo rápido para a rotina e um mais potente quando algo falha."},
	"gui.set.model_load":     {"Loading the models…", "Cargando los modelos…", "Chargement des modèles…", "Lade die Modelle…", "A carregar os modelos…"},
	"gui.set.general":        {"General", "General", "Général", "Allgemein", "Geral"},
	"gui.set.language":       {"Language", "Idioma", "Langue", "Sprache", "Idioma"},
	"gui.set.language_sub":   {"Of this app and of omega's replies.", "De esta app y de las respuestas de omega.", "De cette app et des réponses d'omega.", "Dieser App und der Antworten von omega.", "Desta app e das respostas do omega."},
	"gui.set.auto":           {"Same as the system", "El del sistema", "Celle du système", "Wie das System", "O do sistema"},
	"gui.set.approval":       {"Before running a command", "Antes de ejecutar un comando", "Avant d'exécuter une commande", "Vor dem Ausführen eines Befehls", "Antes de executar um comando"},
	"gui.set.approval_sub":   {"What is blocked for safety never runs, whatever you choose.", "Lo bloqueado por seguridad no se ejecuta nunca, elijas lo que elijas.", "Ce qui est bloqué par sécurité ne s'exécute jamais, quel que soit votre choix.", "Aus Sicherheitsgründen Gesperrtes läuft nie, egal was du wählst.", "O que está bloqueado por segurança nunca é executado, escolha o que escolher."},
	"gui.approval.dangerous": {"Ask only for risky actions", "Preguntar solo lo arriesgado", "Demander seulement pour les actions risquées", "Nur bei riskanten Aktionen fragen", "Perguntar só o que é arriscado"},
	"gui.approval.all":       {"Ask for anything that changes something", "Preguntar todo lo que cambie algo", "Demander pour tout ce qui change quelque chose", "Bei allem fragen, was etwas ändert", "Perguntar tudo o que altere algo"},
	"gui.approval.never":     {"Never ask", "No preguntar nunca", "Ne jamais demander", "Nie fragen", "Nunca perguntar"},
	"gui.set.effort":         {"Reasoning effort", "Esfuerzo de razonamiento", "Effort de réflexion", "Denkaufwand", "Esforço de raciocínio"},
	"gui.set.effort_sub":     {"Claude models only. More effort is slower and costs more.", "Solo modelos Claude. Más esfuerzo es más lento y cuesta más.", "Modèles Claude uniquement. Plus d'effort est plus lent et coûte plus cher.", "Nur Claude-Modelle. Mehr Aufwand ist langsamer und teurer.", "Só modelos Claude. Mais esforço é mais lento e custa mais."},
	"gui.effort.default":     {"Default", "Por defecto", "Par défaut", "Standard", "Predefinido"},
	"gui.effort.low":         {"Low", "Bajo", "Faible", "Niedrig", "Baixo"},
	"gui.effort.medium":      {"Medium", "Medio", "Moyen", "Mittel", "Médio"},
	"gui.effort.high":        {"High", "Alto", "Élevé", "Hoch", "Alto"},
	"gui.effort.xhigh":       {"Very high", "Muy alto", "Très élevé", "Sehr hoch", "Muito alto"},
	"gui.effort.max":         {"Maximum", "Máximo", "Maximum", "Maximal", "Máximo"},
	"gui.set.timeout":        {"Longest a command may run", "Tiempo máximo de un comando", "Durée maximale d'une commande", "Längste Laufzeit eines Befehls", "Tempo máximo de um comando"},
	"gui.set.catalog":        {"Model catalogue", "Catálogo de modelos", "Catalogue des modèles", "Modellkatalog", "Catálogo de modelos"},
	"gui.set.catalog_sub":    {"Download the public list of models and prices that automatic selection uses.", "Descargar la lista pública de modelos y precios que usa la selección automática.", "Télécharger la liste publique des modèles et des prix qu'utilise la sélection automatique.", "Die öffentliche Liste der Modelle und Preise laden, die die automatische Auswahl nutzt.", "Descarregar a lista pública de modelos e preços que a seleção automática usa."},
	"gui.set.file":           {"Configuration file", "Archivo de configuración", "Fichier de configuration", "Konfigurationsdatei", "Ficheiro de configuração"},
	"gui.set.done":           {"Saved", "Guardado", "Enregistré", "Gespeichert", "Guardado"},
}

func init() {
	for k, v := range guiMessages {
		messages[k] = v
	}
}

// WithPrefix returns every message whose key starts with one of the
// prefixes, in the active language. The desktop app hands its frontend the
// texts it shows this way.
func WithPrefix(prefixes ...string) map[string]string {
	out := map[string]string{}
	for k := range messages {
		if strings.Contains(k, "@") {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(k, p) {
				out[k] = T(k)
			}
		}
	}
	return out
}
