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

	"gui.ask":          {"Or say what you need…", "O di qué necesitas…", "Ou dites ce qu'il vous faut…", "Oder sag, was du brauchst…", "Ou diga o que precisa…"},
	"gui.reply":        {"Reply to omega…", "Responde a omega…", "Répondre à omega…", "omega antworten…", "Responder ao omega…"},
	"gui.send":         {"Send", "Enviar", "Envoyer", "Senden", "Enviar"},
	"gui.dry_run":      {"Read only: offer changes, make none", "Solo lectura: propone cambios, no hace ninguno", "Lecture seule : proposer, ne rien changer", "Nur lesen: Änderungen vorschlagen, keine machen", "Só leitura: propõe alterações, não faz nenhuma"},
	"gui.stop":         {"Stop", "Detener", "Arrêter", "Stoppen", "Parar"},
	"gui.new":          {"New session", "Nueva sesión", "Nouvelle session", "Neue Sitzung", "Nova sessão"},
	"gui.working":      {"Working…", "Trabajando…", "En cours…", "Arbeite…", "A trabalhar…"},
	"gui.ended":        {"Session ended", "Sesión terminada", "Session terminée", "Sitzung beendet", "Sessão terminada"},
	"gui.approve":      {"Run", "Ejecutar", "Exécuter", "Ausführen", "Executar"},
	"gui.approve_all":  {"Run, and stop asking", "Ejecutar y no preguntar más", "Exécuter et ne plus demander", "Ausführen, nicht mehr fragen", "Executar e não perguntar mais"},
	"gui.deny":         {"Don't run", "No ejecutar", "Ne pas exécuter", "Nicht ausführen", "Não executar"},
	"gui.choose":       {"Do the selected", "Hacer lo seleccionado", "Faire la sélection", "Auswahl ausführen", "Fazer o selecionado"},
	"gui.choose_none":  {"None of these", "Ninguna", "Aucune", "Nichts davon", "Nenhuma"},
	"gui.password":     {"Password", "Contraseña", "Mot de passe", "Passwort", "Palavra-passe"},
	"gui.password_ok":  {"Continue", "Continuar", "Continuer", "Weiter", "Continuar"},
	"gui.password_no":  {"Skip what needs admin access", "Saltar lo que requiere permisos", "Ignorer ce qui demande des droits", "Admin-Schritte überspringen", "Saltar o que precisa de permissões"},
	"gui.tokens":       {"tokens", "tokens", "jetons", "Tokens", "tokens"},
	"gui.lines":        {"lines", "líneas", "lignes", "Zeilen", "linhas"},
	"gui.risk.normal":  {"normal", "normal", "normal", "normal", "normal"},
	"gui.risk.safe":    {"read-only", "solo lectura", "lecture seule", "nur lesen", "só leitura"},
	"gui.risk.danger":  {"dangerous", "peligroso", "dangereux", "gefährlich", "perigoso"},
	"gui.demo":         {"Demo: nothing is run and no API is called", "Demo: no se ejecuta nada ni se llama a ninguna API", "Démo : rien n'est exécuté, aucune API appelée", "Demo: nichts wird ausgeführt, keine API aufgerufen", "Demo: nada é executado nem é chamada nenhuma API"},
	"gui.start_failed": {"The session could not start", "No se pudo iniciar la sesión", "La session n'a pas pu démarrer", "Die Sitzung konnte nicht starten", "Não foi possível iniciar a sessão"},
	"gui.setup":        {"Provider and key", "Proveedor y clave", "Fournisseur et clé", "Anbieter und Schlüssel", "Fornecedor e chave"},
	"gui.setup_open":   {"Set up", "Configurar", "Configurer", "Einrichten", "Configurar"},
	"gui.setup_intro":  {"Choose who runs the model. The key is checked with the provider and stored in this system's keychain.", "Elige quién ejecuta el modelo. La clave se comprueba con el proveedor y se guarda en el llavero del sistema.", "Choisissez qui exécute le modèle. La clé est vérifiée auprès du fournisseur et rangée dans le trousseau du système.", "Wähle, wer das Modell ausführt. Der Schlüssel wird beim Anbieter geprüft und im Schlüsselbund des Systems abgelegt.", "Escolha quem executa o modelo. A chave é verificada com o fornecedor e guardada no porta-chaves do sistema."},
	"gui.key":          {"API key", "Clave de API", "Clé d'API", "API-Schlüssel", "Chave de API"},
	"gui.key_keep":     {"Leave empty to keep the saved key", "Déjalo vacío para conservar la clave guardada", "Laisser vide pour garder la clé enregistrée", "Leer lassen, um den gespeicherten Schlüssel zu behalten", "Deixe vazio para manter a chave guardada"},
	"gui.get_key":      {"Get a key", "Conseguir una clave", "Obtenir une clé", "Schlüssel besorgen", "Obter uma chave"},
	"gui.more_info":    {"How to install it", "Cómo instalarlo", "Comment l'installer", "So installierst du es", "Como instalar"},
	"gui.base_url":     {"Endpoint URL (OpenAI-compatible)", "URL del endpoint (compatible con OpenAI)", "URL du point d'accès (compatible OpenAI)", "Endpunkt-URL (OpenAI-kompatibel)", "URL do endpoint (compatível com OpenAI)"},
	"gui.base_url_req": {"this provider needs an endpoint URL", "este proveedor necesita la URL del endpoint", "ce fournisseur a besoin de l'URL du point d'accès", "dieser Anbieter braucht eine Endpunkt-URL", "este fornecedor precisa do URL do endpoint"},
	"gui.save":         {"Save", "Guardar", "Enregistrer", "Speichern", "Guardar"},
	"gui.saving":       {"Checking…", "Comprobando…", "Vérification…", "Prüfe…", "A verificar…"},
	"gui.saved":        {"Saved: %s · %s", "Guardado: %s · %s", "Enregistré : %s · %s", "Gespeichert: %s · %s", "Guardado: %s · %s"},
	"gui.back":         {"Back", "Volver", "Retour", "Zurück", "Voltar"},
	"gui.in_use":       {"in use", "en uso", "utilisé", "in Verwendung", "em uso"},
	"gui.not_set":      {"not set up yet", "sin configurar", "pas encore configuré", "noch nicht eingerichtet", "ainda por configurar"},
	"gui.unknown_prov": {"unknown provider: %s", "proveedor desconocido: %s", "fournisseur inconnu : %s", "unbekannter Anbieter: %s", "fornecedor desconhecido: %s"},
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
