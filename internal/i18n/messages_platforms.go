package i18n

import "strings"

// Messages of the Linux support: where API keys are kept, the Linux steps
// and the parts of omega that only exist on macOS. Same order as messages:
// en, es, fr, de, pt.
// (The file is not named after Linux: a _linux.go file only builds there.)
var platformMessages = map[string][]string{
	// ---- where keys are kept ----
	"secrets.src.env": {
		"an environment variable", "una variable de entorno", "une variable d'environnement", "eine Umgebungsvariable", "uma variável de ambiente",
	},
	"secrets.src.keychain": {
		"the macOS Keychain", "el llavero de macOS", "le trousseau macOS", "der macOS-Schlüsselbund", "o porta-chaves do macOS",
	},
	"secrets.src.secret_service": {
		"the system keyring (Secret Service)", "el llavero del sistema (Secret Service)", "le trousseau du système (Secret Service)",
		"der Schlüsselbund des Systems (Secret Service)", "o porta-chaves do sistema (Secret Service)",
	},
	"secrets.src.file": {
		"the file %s, readable only by you (no system keyring answered)",
		"el archivo %s, que solo tú puedes leer (no respondió ningún llavero del sistema)",
		"le fichier %s, lisible par vous seul (aucun trousseau du système n'a répondu)",
		"die Datei %s, nur für dich lesbar (kein Schlüsselbund des Systems hat geantwortet)",
		"o ficheiro %s, que só tu podes ler (nenhum porta-chaves do sistema respondeu)",
	},
	"secrets.unsupported": {
		"omega cannot store keys on %s; export %s instead",
		"omega no puede guardar claves en %s; exporta %s en su lugar",
		"omega ne peut pas enregistrer de clés sous %s ; exportez %s à la place",
		"omega kann unter %s keine Keys speichern; exportiere stattdessen %s",
		"o omega não consegue guardar chaves em %s; exporta %s em vez disso",
	},
	"secrets.delete_service": {
		"deleting from the system keyring: %v: %s", "borrando del llavero del sistema: %v: %s", "suppression du trousseau du système : %v : %s",
		"Löschen aus dem Schlüsselbund des Systems: %v: %s", "a apagar do porta-chaves do sistema: %v: %s",
	},
	"secrets.none": {
		"no key is stored for %s", "no hay ninguna clave guardada para %s", "aucune clé n'est enregistrée pour %s",
		"für %s ist kein Key gespeichert", "não há nenhuma chave guardada para %s",
	},
	"secrets.file": {
		"writing the key file %s: %v", "escribiendo el archivo de claves %s: %v", "écriture du fichier de clés %s : %v",
		"Schreiben der Key-Datei %s: %v", "a escrever o ficheiro de chaves %s: %v",
	},
	"doctor.key_store": {
		"key storage: omega setup would save it to %s", "almacén de claves: omega setup la guardaría en %s",
		"stockage des clés : omega setup l'enregistrerait dans %s", "Key-Speicher: omega setup würde ihn in %s speichern",
		"armazém de chaves: o omega setup guardá-la-ia em %s",
	},

	// ---- the parts that only exist on macOS ----
	"mac.only": {
		"%s only works on macOS: it controls login items, the menu bar, the Dock, shortcuts, privacy permissions and apps. This machine runs %s.",
		"%s solo funciona en macOS: controla los ítems de inicio, la barra de menús, el Dock, los atajos, los permisos de privacidad y las apps. Este equipo usa %s.",
		"%s ne fonctionne que sous macOS : il contrôle les éléments d'ouverture, la barre des menus, le Dock, les raccourcis, les autorisations de confidentialité et les apps. Cette machine utilise %s.",
		"%s funktioniert nur unter macOS: Es steuert Anmeldeobjekte, Menüleiste, Dock, Tastenkürzel, Datenschutzrechte und Apps. Auf diesem Rechner läuft %s.",
		"%s só funciona no macOS: controla os itens de início de sessão, a barra de menus, a Dock, os atalhos, as permissões de privacidade e as apps. Esta máquina usa %s.",
	},
	"mac.only_slash": {
		"%s only works on macOS", "%s solo funciona en macOS", "%s ne fonctionne que sous macOS", "%s funktioniert nur unter macOS", "%s só funciona no macOS",
	},

	// ---- texts that name the Mac, as they read on any other system ----
	"cli.short@other": {
		"AI-driven system maintenance: updates everything installed and cleans up what is left over",
		"Mantenimiento del sistema guiado por IA: actualiza todo lo instalado y limpia lo que sobra",
		"Maintenance du système pilotée par IA : met à jour tout ce qui est installé et nettoie ce qui reste",
		"KI-gesteuerte Systemwartung: aktualisiert alles Installierte und räumt Überflüssiges auf",
		"Manutenção do sistema guiada por IA: atualiza tudo o que está instalado e limpa o que sobra",
	},
	"ui.welcome_tagline@other": {
		"AI maintenance for your machine", "Mantenimiento con IA para tu equipo", "Maintenance par IA pour votre machine",
		"KI-Wartung für deinen Rechner", "Manutenção com IA para a sua máquina",
	},
	"sudo.intro@other": {
		"omega needs sudo for some steps (e.g. system package updates).",
		"omega necesita sudo para algunos pasos (p. ej. actualizar los paquetes del sistema).",
		"omega a besoin de sudo pour certaines étapes (p. ex. mettre à jour les paquets du système).",
		"omega benötigt sudo für einige Schritte (z. B. Updates der Systempakete).",
		"o omega precisa de sudo para alguns passos (p. ex. atualizar os pacotes do sistema).",
	},
	"cmd.steps@other": {
		"List the maintenance steps detected on this machine", "Listar los pasos de mantenimiento detectados", "Lister les étapes de maintenance détectées",
		"Erkannte Wartungsschritte auflisten", "Listar os passos de manutenção detetados",
	},
	"cmd.notes@other": {
		"Show what omega has learned about this machine", "Ver lo que omega ha aprendido de esta máquina", "Voir ce qu'omega a appris sur cette machine",
		"Anzeigen, was omega über diesen Rechner gelernt hat", "Ver o que o omega aprendeu sobre esta máquina",
	},
	"model.free@other": {
		"free, runs on this machine", "gratis, se ejecuta en este equipo", "gratuit, tourne sur cette machine", "kostenlos, läuft auf diesem Rechner", "grátis, corre nesta máquina",
	},
	"setup.local_found@other": {
		"running on this machine", "en marcha en este equipo", "actif sur cette machine", "läuft auf diesem Rechner", "a correr nesta máquina",
	},

	// ---- Linux step names ----
	"step.apt-outdated":     {"Outdated apt packages", "Paquetes de apt desactualizados", "Paquets apt obsolètes", "Veraltete apt-Pakete", "Pacotes apt desatualizados"},
	"step.dnf-outdated":     {"Outdated dnf packages", "Paquetes de dnf desactualizados", "Paquets dnf obsolètes", "Veraltete dnf-Pakete", "Pacotes dnf desatualizados"},
	"step.pacman-outdated":  {"Outdated pacman packages", "Paquetes de pacman desactualizados", "Paquets pacman obsolètes", "Veraltete pacman-Pakete", "Pacotes pacman desatualizados"},
	"step.zypper-outdated":  {"Outdated zypper packages", "Paquetes de zypper desactualizados", "Paquets zypper obsolètes", "Veraltete zypper-Pakete", "Pacotes zypper desatualizados"},
	"step.flatpak-outdated": {"Outdated Flatpak apps", "Apps de Flatpak desactualizadas", "Apps Flatpak obsolètes", "Veraltete Flatpak-Apps", "Apps Flatpak desatualizadas"},
	"step.snap-outdated":    {"Outdated snaps", "Snaps desactualizados", "Snaps obsolètes", "Veraltete Snaps", "Snaps desatualizados"},
	"step.journal-size":     {"Journal size", "Tamaño del journal", "Taille du journal", "Größe des Journals", "Tamanho do journal"},
	"step.apt":              {"apt packages", "Paquetes de apt", "Paquets apt", "apt-Pakete", "Pacotes apt"},
	"step.dnf":              {"dnf packages", "Paquetes de dnf", "Paquets dnf", "dnf-Pakete", "Pacotes dnf"},
	"step.pacman":           {"pacman packages", "Paquetes de pacman", "Paquets pacman", "pacman-Pakete", "Pacotes pacman"},
	"step.zypper":           {"zypper packages", "Paquetes de zypper", "Paquets zypper", "zypper-Pakete", "Pacotes zypper"},
	"step.zypper-dup":       {"zypper distribution upgrade", "Actualización de distribución con zypper", "Mise à niveau de la distribution avec zypper", "zypper-Distributionsupgrade", "Atualização da distribuição com zypper"},
	"step.flatpak":          {"Flatpak apps", "Apps de Flatpak", "Apps Flatpak", "Flatpak-Apps", "Apps Flatpak"},
	"step.snap":             {"snaps", "Snaps", "Snaps", "Snaps", "Snaps"},
	"step.apt-clean":        {"apt package cache", "Caché de paquetes de apt", "Cache des paquets apt", "apt-Paketcache", "Cache de pacotes do apt"},
	"step.apt-autoremove":   {"Orphaned apt packages", "Paquetes huérfanos de apt", "Paquets apt orphelins", "Verwaiste apt-Pakete", "Pacotes apt órfãos"},
	"step.dnf-clean":        {"dnf cache", "Caché de dnf", "Cache dnf", "dnf-Cache", "Cache do dnf"},
	"step.dnf-autoremove":   {"Orphaned dnf packages", "Paquetes huérfanos de dnf", "Paquets dnf orphelins", "Verwaiste dnf-Pakete", "Pacotes dnf órfãos"},
	"step.pacman-cache":     {"pacman package cache", "Caché de paquetes de pacman", "Cache des paquets pacman", "pacman-Paketcache", "Cache de pacotes do pacman"},
	"step.pacman-orphans":   {"Orphaned pacman packages", "Paquetes huérfanos de pacman", "Paquets pacman orphelins", "Verwaiste pacman-Pakete", "Pacotes pacman órfãos"},
	"step.zypper-clean":     {"zypper cache", "Caché de zypper", "Cache zypper", "zypper-Cache", "Cache do zypper"},
	"step.flatpak-unused":   {"Unused Flatpak runtimes", "Runtimes de Flatpak sin usar", "Runtimes Flatpak inutilisés", "Ungenutzte Flatpak-Laufzeiten", "Runtimes Flatpak não usados"},
	"step.snap-old":         {"Old snap revisions", "Revisiones antiguas de snaps", "Anciennes révisions de snaps", "Alte Snap-Revisionen", "Revisões antigas de snaps"},
	"step.journal-vacuum":   {"Old journal entries", "Entradas antiguas del journal", "Anciennes entrées du journal", "Alte Journal-Einträge", "Entradas antigas do journal"},
	"step.thumbnail-cache":  {"Thumbnail cache", "Caché de miniaturas", "Cache des vignettes", "Vorschaubild-Cache", "Cache de miniaturas"},
	"step.playwright-cache": {"Playwright browsers", "Navegadores de Playwright", "Navigateurs Playwright", "Playwright-Browser", "Navegadores do Playwright"},
}

// macPhrases are the words of the long help that name the Mac and its
// tools, with what replaces them elsewhere, per language.
var macPhrases = [][2][]string{
	{
		{"to look after your Mac:", "para cuidar tu Mac:", "pour entretenir votre Mac :", "um deinen Mac zu pflegen:", "para cuidar do seu Mac:"},
		{"to look after your machine:", "para cuidar tu equipo:", "pour entretenir votre machine :", "um deinen Rechner zu pflegen:", "para cuidar da sua máquina:"},
	},
}

func init() {
	for k, v := range platformMessages {
		messages[k] = v
	}
	// The long help is the same text with the Mac's name taken out.
	long := append([]string(nil), messages["cli.long"]...)
	for _, phrase := range macPhrases {
		for i := range long {
			long[i] = strings.Replace(long[i], phrase[0][i], phrase[1][i], 1)
		}
	}
	messages["cli.long@other"] = long
}
