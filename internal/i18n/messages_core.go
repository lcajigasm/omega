package i18n

// Messages of the core packages: configuration checks, pending items and
// restore points. Same order as messages: en, es, fr, de, pt.
var coreMessages = map[string][]string{
	"config.bad_budget": {
		"disk_budgets.%s: %q is not a size: write it as 500MB or 8GB",
		"disk_budgets.%s: %q no es un tamaño: escríbelo como 500MB u 8GB",
		"disk_budgets.%s : %q n'est pas une taille : écrivez-la comme 500MB ou 8GB",
		"disk_budgets.%s: %q ist keine Größe: schreib sie als 500MB oder 8GB",
		"disk_budgets.%s: %q não é um tamanho: escreve-o como 500MB ou 8GB",
	},
	"pending.not_found": {
		"no pending item %q (omega pending --all lists them)",
		"no hay ningún pendiente %q (omega pending --all los lista)",
		"aucun élément en attente %q (omega pending --all les liste)",
		"kein offener Punkt %q (omega pending --all listet sie auf)",
		"não há nenhum pendente %q (omega pending --all lista-os)",
	},
	"pending.ambiguous": {
		"%q matches more than one pending item (%s, %s…): write more of it",
		"%q coincide con más de un pendiente (%s, %s…): escribe más",
		"%q correspond à plusieurs éléments en attente (%s, %s…) : écrivez-en davantage",
		"%q passt zu mehr als einem offenen Punkt (%s, %s…): schreib mehr davon",
		"%q corresponde a mais de um pendente (%s, %s…): escreve mais",
	},
	"pending.st_stale": {
		"stale: not seen for 30 days",
		"caducado: sin verse en 30 días",
		"périmé : pas revu depuis 30 jours",
		"veraltet: seit 30 Tagen nicht gesehen",
		"caducado: não visto há 30 dias",
	},
	"pending.bad_period": {
		"%q is not a period: use 3d, 2w or 1m",
		"%q no es un plazo: usa 3d, 2w o 1m",
		"%q n'est pas une durée : utilisez 3d, 2w ou 1m",
		"%q ist kein Zeitraum: nimm 3d, 2w oder 1m",
		"%q não é um prazo: usa 3d, 2w ou 1m",
	},
	"snapshots.none": {
		"There are no snapshots on this system yet: nothing to restore.",
		"Todavía no hay instantáneas en este sistema: no hay nada que restaurar.",
		"Il n'y a encore aucun instantané sur ce système : rien à restaurer.",
		"Auf diesem System gibt es noch keine Snapshots: nichts wiederherzustellen.",
		"Ainda não há instantâneos neste sistema: nada para restaurar.",
	},
	"snapshots.unsupported": {
		"This system has no snapshots omega can list (on Linux, omega reads them with snapper).",
		"Este sistema no tiene instantáneas que omega pueda listar (en Linux, omega las lee con snapper).",
		"Ce système n'a aucun instantané qu'omega sache lister (sous Linux, omega les lit avec snapper).",
		"Dieses System hat keine Snapshots, die omega auflisten kann (unter Linux liest omega sie mit snapper).",
		"Este sistema não tem instantâneos que o omega possa listar (no Linux, o omega lê-os com o snapper).",
	},
	"snapshots.needs_sudo": {
		"listing the snapper snapshots needs the administrator password, and omega does not wait at a password prompt: run sudo snapper list yourself",
		"listar las instantáneas de snapper necesita la contraseña de administrador, y omega no espera en una petición de contraseña: ejecuta tú sudo snapper list",
		"lister les instantanés snapper demande le mot de passe administrateur, et omega n'attend pas devant une demande de mot de passe : lancez vous-même sudo snapper list",
		"die snapper-Snapshots aufzulisten braucht das Administratorpasswort, und omega wartet nicht an einer Passwortabfrage: führe sudo snapper list selbst aus",
		"listar os instantâneos do snapper precisa da palavra-passe de administrador, e o omega não espera num pedido de palavra-passe: executa tu sudo snapper list",
	},
	"snapshots.available": {
		"Snapshots on this system: %d",
		"Instantáneas en este sistema: %d",
		"Instantanés sur ce système : %d",
		"Snapshots auf diesem System: %d",
		"Instantâneos neste sistema: %d",
	},
	"snapshots.restore_apfs": {
		"omega only lists them. To restore one, start macOS Recovery and choose Restore from Time Machine.",
		"omega solo las lista. Para restaurar una, arranca la Recuperación de macOS y elige Restaurar desde Time Machine.",
		"omega se contente de les lister. Pour en restaurer un, démarrez la récupération macOS et choisissez Restaurer à partir de Time Machine.",
		"omega listet sie nur auf. Um einen wiederherzustellen, starte die macOS-Wiederherstellung und wähle Aus Time Machine wiederherstellen.",
		"O omega só os lista. Para restaurar um, arranca a Recuperação do macOS e escolhe Restaurar a partir do Time Machine.",
	},
	"snapshots.restore_windows": {
		"omega only lists them. To restore one, run rstrui.exe (System Restore).",
		"omega solo los lista. Para restaurar uno, ejecuta rstrui.exe (Restaurar sistema).",
		"omega se contente de les lister. Pour en restaurer un, lancez rstrui.exe (Restauration du système).",
		"omega listet sie nur auf. Um einen wiederherzustellen, starte rstrui.exe (Systemwiederherstellung).",
		"O omega só os lista. Para restaurar um, executa rstrui.exe (Restauro do Sistema).",
	},
	"snapshots.restore_snapper": {
		"omega only lists them. To undo the changes since one, run sudo snapper undochange <id>..0.",
		"omega solo las lista. Para deshacer los cambios desde una, ejecuta sudo snapper undochange <id>..0.",
		"omega se contente de les lister. Pour annuler les changements depuis l'un d'eux, lancez sudo snapper undochange <id>..0.",
		"omega listet sie nur auf. Um die Änderungen seit einem rückgängig zu machen, führe sudo snapper undochange <id>..0 aus.",
		"O omega só os lista. Para desfazer as alterações desde um, executa sudo snapper undochange <id>..0.",
	},
}

func init() {
	for k, v := range coreMessages {
		messages[k] = v
	}
}
