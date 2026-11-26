package i18n

// Messages of omega ultra. Same order as messages: en, es, fr, de, pt.
var ultraMessages = map[string][]string{
	"cmd.ultra": {
		"Everything omega does, deeper, asking before every change",
		"Todo lo que hace omega, a fondo, preguntando cada cambio",
		"Tout ce que fait omega, à fond, avec accord à chaque pas",
		"Alles von omega, gründlicher, mit Rückfrage bei allem",
		"Tudo o que o omega faz, a fundo, perguntando cada passo",
	},
	"cmd.ultra_long": {
		"The deepest pass omega can make, attended from start to end. It runs every stage of omega full (setup, report, health, updates, cleanup, Docker, tool configuration, developer setup, the Mac, repairs, earlier runs) and two only ultra has: a deep clean of what the cleaning steps never reach (the git worktrees AI agents leave, the files their runs write next to your projects, Docker images, containers and volumes one by one, build folders of projects idle two weeks, merged branches, disk limits) and a pass through every review omega has (AI agents left running, configuration assistants, SSH keys and secrets in histories, privacy, network, skills). The stages start on the provider's smart model with high effort and move to its most capable one when a step fails; the deep clean starts on the most capable one (Anthropic: Sonnet, then Opus; set another with providers.<name>.ultra_model). The model must show the evidence for every change it offers, each deletion is checked again just before it runs, a worktree's uncommitted work is saved first, and every change is put to you: --yes and --auto are refused. It costs more than omega full.",
		"La pasada más profunda que puede hacer omega, contigo delante de principio a fin. Hace todas las etapas de omega full (configuración, informe, salud, actualizaciones, limpieza, Docker, configuración de herramientas, entorno de desarrollo, el Mac, reparaciones, ejecuciones anteriores) y dos que solo tiene ultra: una limpieza profunda de lo que los pasos de limpieza nunca alcanzan (los worktrees de git que dejan los agentes de IA, los ficheros que sus ejecuciones escriben junto a tus proyectos, las imágenes, contenedores y volúmenes de Docker uno a uno, las carpetas de compilación de proyectos parados dos semanas, las ramas fusionadas, los límites de disco) y un repaso de todas las revisiones de omega (agentes de IA que siguen ejecutándose, asistentes de configuración, claves SSH y secretos en historiales, privacidad, red, skills). Las etapas empiezan con el modelo inteligente del proveedor y esfuerzo alto, y pasan al más capaz cuando un paso falla; la limpieza profunda empieza en el más capaz (Anthropic: Sonnet y luego Opus; elige otro con providers.<nombre>.ultra_model). El modelo tiene que mostrar la evidencia de cada cambio que propone, cada borrado se vuelve a comprobar justo antes de hacerse, el trabajo sin confirmar de un worktree se guarda antes, y cada cambio se te pregunta: --yes y --auto se rechazan. Cuesta más que omega full.",
		"Le passage le plus profond que fait omega, en votre présence du début à la fin. Il fait toutes les étapes d'omega full et deux propres à ultra : un nettoyage profond de ce que les étapes de nettoyage n'atteignent jamais (les worktrees git laissés par les agents d'IA, les fichiers que leurs exécutions écrivent à côté de vos projets, les images, conteneurs et volumes Docker un par un, les dossiers de build des projets inactifs depuis deux semaines, les branches fusionnées, les limites de disque) et un passage par toutes les revues d'omega (agents d'IA encore actifs, assistants de configuration, clés SSH et secrets dans les historiques, confidentialité, réseau, skills). Les étapes démarrent sur le modèle intelligent du fournisseur avec un effort élevé et passent au plus capable quand une étape échoue ; le nettoyage profond démarre sur le plus capable. Le modèle doit montrer la preuve de chaque changement proposé, chaque suppression est revérifiée juste avant, le travail non validé d'un worktree est sauvegardé d'abord, et chaque changement vous est demandé : --yes et --auto sont refusés. Cela coûte plus qu'omega full.",
		"Der gründlichste Durchlauf, den omega machen kann, von Anfang bis Ende mit Ihnen. Er macht alle Etappen von omega full und zwei, die nur ultra hat: eine Tiefenreinigung dessen, was die Reinigungsschritte nie erreichen (git-Worktrees, die KI-Agenten zurücklassen, Dateien, die ihre Läufe neben Ihre Projekte schreiben, Docker-Images, -Container und -Volumes einzeln, Build-Ordner seit zwei Wochen ruhender Projekte, gemergte Branches, Speicherlimits) und einen Durchgang durch alle Prüfungen von omega (noch laufende KI-Agenten, Konfigurationsassistenten, SSH-Schlüssel und Geheimnisse in Verläufen, Datenschutz, Netzwerk, Skills). Die Etappen beginnen mit dem starken Modell des Anbieters und hohem Aufwand und wechseln zum fähigsten, wenn ein Schritt fehlschlägt; die Tiefenreinigung beginnt mit dem fähigsten. Das Modell muss für jede vorgeschlagene Änderung den Beleg zeigen, jede Löschung wird direkt davor erneut geprüft, nicht committete Arbeit eines Worktrees wird zuerst gesichert, und jede Änderung wird Ihnen vorgelegt: --yes und --auto werden abgelehnt. Es kostet mehr als omega full.",
		"A passagem mais profunda que o omega consegue fazer, consigo presente do início ao fim. Faz todas as etapas do omega full e duas que só o ultra tem: uma limpeza profunda do que os passos de limpeza nunca alcançam (os worktrees git que os agentes de IA deixam, os ficheiros que as suas execuções escrevem junto aos seus projetos, imagens, contentores e volumes Docker um a um, pastas de build de projetos parados há duas semanas, ramos fundidos, limites de disco) e uma passagem por todas as revisões do omega (agentes de IA ainda a correr, assistentes de configuração, chaves SSH e segredos em históricos, privacidade, rede, skills). As etapas começam no modelo inteligente do fornecedor com esforço alto e passam ao mais capaz quando um passo falha; a limpeza profunda começa no mais capaz. O modelo tem de mostrar a evidência de cada alteração que propõe, cada remoção é verificada de novo mesmo antes, o trabalho por confirmar de um worktree é guardado primeiro, e cada alteração é-lhe perguntada: --yes e --auto são recusados. Custa mais do que o omega full.",
	},
	"ultra.title": {"Ultra run", "Recorrido ultra", "Parcours ultra", "Ultra-Durchlauf", "Percurso ultra"},
	"ultra.plan_intro": {
		"omega ultra: every stage, deeper, on stronger models. Nothing changes without your yes, and every option shows what verified it. Untick what you do not want.",
		"omega ultra: todas las etapas, más a fondo, con modelos más potentes. Nada cambia sin tu sí, y cada opción muestra qué la ha verificado. Desmarca lo que no quieras.",
		"omega ultra : toutes les étapes, plus en profondeur, avec des modèles plus puissants. Rien ne change sans votre accord, et chaque option montre ce qui l'a vérifiée. Décochez ce que vous ne voulez pas.",
		"omega ultra: alle Etappen, gründlicher, mit stärkeren Modellen. Nichts ändert sich ohne Ihr Ja, und jede Option zeigt, was sie belegt. Entfernen Sie den Haken bei dem, was Sie nicht wollen.",
		"omega ultra: todas as etapas, mais a fundo, com modelos mais potentes. Nada muda sem o seu sim, e cada opção mostra o que a verificou. Desmarque o que não quiser.",
	},
	"ultra.flag_refused": {
		"omega ultra asks before every change: %s is not available in it",
		"omega ultra pregunta antes de cada cambio: %s no está disponible en él",
		"omega ultra demande avant chaque changement : %s n'y est pas disponible",
		"omega ultra fragt vor jeder Änderung: %s ist dort nicht verfügbar",
		"o omega ultra pergunta antes de cada alteração: %s não está disponível nele",
	},
	"ultra.need_tty": {
		"omega ultra needs a terminal: a person confirms every change. For runs nobody watches, use omega full --unattended.",
		"omega ultra necesita un terminal: una persona confirma cada cambio. Para ejecuciones sin nadie delante, usa omega full --unattended.",
		"omega ultra a besoin d'un terminal : une personne confirme chaque changement. Pour une exécution sans surveillance, utilisez omega full --unattended.",
		"omega ultra braucht ein Terminal: ein Mensch bestätigt jede Änderung. Für unbeaufsichtigte Läufe: omega full --unattended.",
		"o omega ultra precisa de um terminal: uma pessoa confirma cada alteração. Para execuções sem ninguém, use omega full --unattended.",
	},
	"full.s.deep.title": {"Deep clean", "Limpieza profunda", "Nettoyage profond", "Tiefenreinigung", "Limpeza profunda"},
	"full.s.deep.what": {
		"Finds what the cleaning steps never reach: worktrees AI agents left, their run files, Docker object by object, build folders, merged branches, folders over their limit; inspects each one and removes what you pick.",
		"Busca lo que los pasos de limpieza nunca alcanzan: worktrees que dejaron los agentes de IA, sus ficheros de ejecución, Docker objeto a objeto, carpetas de compilación, ramas fusionadas, carpetas por encima de su límite; inspecciona cada cosa y quita lo que elijas.",
		"Trouve ce que les étapes de nettoyage n'atteignent jamais : worktrees laissés par les agents d'IA, leurs fichiers d'exécution, Docker objet par objet, dossiers de build, branches fusionnées, dossiers au-delà de leur limite ; inspecte chacun et supprime ce que vous choisissez.",
		"Findet, was die Reinigungsschritte nie erreichen: von KI-Agenten zurückgelassene Worktrees, ihre Laufdateien, Docker Objekt für Objekt, Build-Ordner, gemergte Branches, Ordner über ihrem Limit; prüft jedes und entfernt, was Sie wählen.",
		"Encontra o que os passos de limpeza nunca alcançam: worktrees deixados por agentes de IA, os seus ficheiros de execução, Docker objeto a objeto, pastas de build, ramos fundidos, pastas acima do limite; inspeciona cada um e remove o que escolher.",
	},
	"full.s.deep.why": {
		"On a developer's machine most of the lost space is there: a dozen 1.7 GB worktrees whose only uncommitted file was the agent's own plan. Each one is checked again before it goes, and any work in it is saved first.",
		"En una máquina de desarrollo casi todo el espacio perdido está ahí: una docena de worktrees de 1,7 GB cuyo único fichero sin confirmar era el plan del propio agente. Cada uno se vuelve a comprobar antes de quitarlo, y el trabajo que tenga se guarda antes.",
		"Sur une machine de développeur, l'essentiel de l'espace perdu est là : une douzaine de worktrees de 1,7 Go dont le seul fichier non validé était le plan de l'agent. Chacun est revérifié avant de partir, et tout travail qu'il contient est sauvegardé d'abord.",
		"Auf einem Entwicklerrechner liegt dort der meiste verlorene Platz: ein Dutzend Worktrees zu 1,7 GB, deren einzige nicht committete Datei der Plan des Agenten war. Jeder wird vorher erneut geprüft, und Arbeit darin wird zuerst gesichert.",
		"Numa máquina de desenvolvimento quase todo o espaço perdido está aí: uma dúzia de worktrees de 1,7 GB cujo único ficheiro por confirmar era o plano do próprio agente. Cada um é verificado de novo antes de sair, e o trabalho que tenha é guardado primeiro.",
	},
	"full.s.care.title": {"Every review", "Todas las revisiones", "Toutes les revues", "Alle Prüfungen", "Todas as revisões"},
	"full.s.care.what": {
		"AI agents left running, the configuration assistants of every tool, SSH keys and secrets in histories, privacy, network and skills: each finding explained, verified and fixed if you pick it.",
		"Agentes de IA que siguen ejecutándose, los asistentes de configuración de cada herramienta, claves SSH y secretos en historiales, privacidad, red y skills: cada hallazgo explicado, verificado y corregido si lo eliges.",
		"Agents d'IA encore actifs, assistants de configuration de chaque outil, clés SSH et secrets dans les historiques, confidentialité, réseau et skills : chaque constat expliqué, vérifié et corrigé si vous le choisissez.",
		"Noch laufende KI-Agenten, die Konfigurationsassistenten jedes Werkzeugs, SSH-Schlüssel und Geheimnisse in Verläufen, Datenschutz, Netzwerk und Skills: jeder Befund erklärt, geprüft und behoben, wenn Sie ihn wählen.",
		"Agentes de IA ainda a correr, os assistentes de configuração de cada ferramenta, chaves SSH e segredos em históricos, privacidade, rede e skills: cada achado explicado, verificado e corrigido se o escolher.",
	},
	"full.s.care.why": {
		"omega has these reviews as separate commands that nobody remembers to run; ultra runs them all and acts on what they find.",
		"omega tiene estas revisiones como órdenes sueltas que nadie se acuerda de ejecutar; ultra las ejecuta todas y actúa sobre lo que encuentran.",
		"omega a ces revues sous forme de commandes séparées que personne ne pense à lancer ; ultra les lance toutes et agit sur ce qu'elles trouvent.",
		"omega hat diese Prüfungen als einzelne Befehle, an die niemand denkt; ultra führt sie alle aus und handelt nach ihren Befunden.",
		"o omega tem estas revisões como comandos soltos que ninguém se lembra de correr; o ultra corre-as todas e age sobre o que encontram.",
	},
}

func init() {
	for k, v := range ultraMessages {
		messages[k] = v
	}
}
