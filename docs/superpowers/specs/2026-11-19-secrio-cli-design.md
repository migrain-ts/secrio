# Secrio CLI — Design (short)

Date : 2026-11-19
Statut : en attente d'approbation
Base : `2026-11-19-secrio-design.md` (§2 rôles, §7 comportements) et le Core déjà dans `main` (`internal/core`, `internal/app`).

## 1. Intention et pourquoi

Donner une première interface utilisable de Secrio : un binaire `secrio` en Go qui permet de créer un coffre, ses projets et ses envs, d'y mettre des secrets (comme des GitHub secrets : projets, envs, paires nom/valeur) et de les récupérer ou de les injecter dans un process. Le Desktop n'existe pas encore, donc la CLI porte aussi l'amorçage (décision de l'utilisateur : option B).

## 2. Périmètre (décidé avec l'utilisateur)

Commandes : `vault create`, `unlock`, `lock`, `project create`, `env create`, `set`, `get`, `list`, `run`. Rien d'autre côté utilisateur : pas d'import/export, d'edit, de delete, d'historique, de comparaison, de changement de mot de passe.

Mes ajouts (à vérifier) : `vault list` (sinon on ne découvre pas les coffres ; lecture seule), `--version`, et `secrio completion` (fourni par Cobra : Bash, Zsh, PowerShell, fish).

**Goals** : un seul binaire sans CGO ; démarrage rapide (Argon2 seulement à `unlock`/`vault create`) ; aucune logique métier dans la CLI (tout passe par `core.New` et `app.Service`) ; utilisable dans un script ou une CI sans terminal.

**Non-goals** : tout le reste du Core (cf. ci-dessus), sync, Desktop, config globale.

**Anti-goals** : un secret dans argv, l'historique du shell, les logs ou un message d'erreur ; une invite interactive qui bloque une CI (sans TTY on échoue vite et clairement) ; un menu interactif.

## 3. Approches considérées (choix de l'utilisateur : « tout me va »)

- Framework : **Cobra (choisi)** ; stdlib seule (plus léger mais aide, sous-commandes, complétion à écrire) ; Cobra + TUI (écarté).
- `run` : **processus enfant avec relais de signaux et propagation du code de sortie (choisi)** ; remplacement du process par `exec` sur Unix (écarté : deux chemins de code avec Windows, difficile à tester).

## 4. Comportement

### Résolution du contexte
- Coffre/projet/env viennent, par priorité : flags `--vault`, `--project`, `--env` ; variables `SECRIO_VAULT`, `SECRIO_PROJECT`, `SECRIO_ENV` ; fichier **`.secrio.toml`** (clés `vault`, `project`, `env`) cherché depuis le répertoire courant vers la racine (comme git). Il ne contient aucun secret et peut être versionné.
- Noms → ids par appels du Core (`ListVaults`, `ListProjects`, `ListEnvironments`). Un nom inconnu : erreur claire. `vault create` refuse un nom déjà pris (la CLI le vérifie ; le Core ne l'impose pas).
- Noms de coffres : lettres, chiffres, `-`, `_`, `.` (le nom sert de nom de fichier via `core.VaultPath`), 1 à 64 caractères.

### Commandes
```
secrio vault create <nom>                 mot de passe saisi masqué + confirmation (ou SECRIO_PASSWORD)
secrio vault list
secrio unlock [<vault>]                   --ttl 12h --idle 1h (défauts) ; mot de passe masqué ou SECRIO_PASSWORD
secrio lock [<vault>]
secrio project create <nom>
secrio env create <nom>                   projet pris dans le contexte
secrio set <NOM>                          valeur : saisie masquée (TTY) ou stdin (un \n final retiré) ; jamais en argument
secrio list                               selon la profondeur du contexte : projets, envs ou noms de secrets (jamais de valeurs)
secrio get <NOM>                          valeur brute sur stdout ; \n final seulement si stdout est un TTY
secrio run [--env X] -- <cmd> [args]      injecte les secrets de l'env, propage le code de sortie
```
- `set` utilise le type `text` et pas de description (YAGNI).
- Quand le coffre n'est pas déverrouillé : si `SECRIO_PASSWORD` est défini, la commande ouvre sans session (`OpenWithPassword`, chemin CI) ; sinon erreur « coffre verrouillé, lancez `secrio unlock` » avec code de sortie 3.
- `run` : variables du process parent préservées, remplacées par les secrets de même nom (`ProcessEnv`) ; Ctrl-C/SIGTERM relayés à l'enfant ; code de sortie de l'enfant renvoyé ; échecs de Secrio avant le lancement : 125 ; commande introuvable 127 ; non exécutable 126.
- Codes de sortie (hors `run`) : 0 succès, 1 erreur, 2 usage, 3 verrouillé/mot de passe invalide.
- Dossier de données : `core.DefaultDataDir()`, surchargeable par `SECRIO_HOME`.

## 5. Contraintes et architecture

- `cmd/secrio/main.go` minimal ; `internal/cli` : un fichier par commande ; entrées/sorties, prompt de mot de passe, environnement et horloge injectés (testabilité) ; la CLI importe `core` et `app` mais jamais `infra`.
- Dépendances ajoutées (mon choix) : `github.com/spf13/cobra`, `golang.org/x/term` (saisie masquée, Windows compris), `github.com/pelletier/go-toml/v2` (lecture de `.secrio.toml`).
- Messages d'erreur : jamais de valeur, de mot de passe ni de nom de secret fautif dans les erreurs (hérité du Core).
- Tests : exécution en mémoire des commandes Cobra (Core réel dans un dossier temporaire, KDF léger, `keychain.NewMemory`, horloge factice) ; quelques tests de sous-processus sur le binaire compilé (propagation du code de sortie de `run`, échec sans TTY, `get` brut quand stdout est un pipe).

## 6. Laissé au builder

Disposition exacte des fichiers de `internal/cli`, textes d'aide, détail de la recherche de `.secrio.toml`, format précis des messages, version injectée via ldflags, choix de la gestion des signaux sous Windows.

## 7. Décisions prises par le rédacteur (à vérifier)

`vault list`, `--version`, complétion ; durées 12 h/1 h ; `list` hiérarchique ; `\n` final conditionnel de `get` ; variables `SECRIO_*` ; codes de sortie ; limites de nom de coffre ; choix de `go-toml/v2` et `x/term` ; `set` sans type ni description.
