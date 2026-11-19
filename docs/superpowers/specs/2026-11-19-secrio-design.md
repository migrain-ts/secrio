# Secrio — Design

Date : 2026-11-19
Statut : brouillon, en attente d'approbation
Base : `specs.txt` (Envault, renommé Secrio), amendé par la discussion ci-dessous.

## 1. Intention et pourquoi

Secrio est un gestionnaire de secrets local-first pour développeurs : une app desktop et une CLI partageant un même cœur Go et un même format de coffre.

Pourquoi : remplacer les fichiers `.env` dispersés par un coffre chiffré, rapide à utiliser, sans compte ni cloud obligatoire. À terme, les pipelines CI/CD doivent pouvoir récupérer les secrets d'un environnement précis (staging, production) sans accéder aux autres.

Critères prioritaires : **rapidité** et **légèreté** (binaires petits, démarrage CLI quasi instantané), sécurité, simplicité.

## 2. Répartition des rôles

| Interface | Rôle | Capacités v1 |
|---|---|---|
| Desktop (Wails) | Écriture et gestion | Créer coffres, projets, environnements ; CRUD des secrets ; comparer les envs ; import/export ; historique ; réglages |
| CLI (`secrio`) | Lecture et exécution, **sans écriture** | `unlock`, `lock`, `get`, `list`, `run`, `vault list` |

La CLI ne contient aucun `set`, `edit`, `delete`, `import` en v1 (décision explicite). `export` : voir « Laissé au builder ».

## 3. Goals / Non-goals / Anti-goals

**Goals**
- Un seul cœur Go, source unique de vérité ; aucune logique métier ni crypto dans les interfaces.
- Plusieurs coffres, projets, environnements ; comparaison d'envs sans exposer les valeurs.
- `secrio run -- <cmd>` injecte les secrets dans un processus enfant, sans fichier en clair, et propage le code de sortie.
- Multiplateforme : Windows, macOS, Linux.

**Non-goals (hors v1)**
- Synchronisation, serveur self-hosted, partage de coffres.
- Intégrations GitHub, GitLab, Docker.
- Rotation de clés, type de secret `File`.
- Écriture depuis la CLI.
- i18n au-delà de l'anglais (structure prévue seulement).

**Anti-goals** (échec même si ça « marche »)
- Un secret ou un nom de secret lisible sur le disque, dans un log ou dans une erreur.
- Un démarrage CLI perceptiblement lent (dérivation Argon2id à chaque appel).
- Une dépendance obligatoire à un service externe.
- Des abstractions artificielles qui alourdissent le code.

## 4. Approches considérées

### Accès à la clé depuis la CLI
1. Mot de passe à chaque appel : simple, sûr, mais ~0,5 s par appel (Argon2id).
2. **Session dans le keychain de l'OS avec TTL — choisi.** Rapide, sans démon. Dépend de l'OS ; repli sur 1 (CI, Linux sans keychain).
3. Démon local : très rapide, mais plus lourd et surface d'attaque en plus.

### Stockage
A. SQLite, seules les valeurs chiffrées : recherche SQL simple, mais noms de clés/projets lisibles sur disque.
B. **SQLite, noms et valeurs chiffrés — choisi.** Aucune métadonnée en clair ; recherche et comparaison en mémoire après déverrouillage ; WAL pour accès concurrent desktop/CLI ; historique en tables.
C. Fichier chiffré unique : le plus léger, mais réécriture complète, pas de concurrence sûre, historique et migrations pénibles.

SQLite via `modernc.org/sqlite` (Go pur, sans CGO) pour garder la CLI autonome et le cross-compile simple.

### Accès CI/CD (futur)
1. Serveur self-hosted (binaire Go, SQLite, ne stocke que du chiffré, tokens de service en lecture seule).
2. Bundle chiffré exporté + clé fournie via un secret natif de la CI (type sops/age) : chemin intermédiaire sans infra.
3. SaaS hébergé : écarté (contredit le positionnement).

Décision v1 : local seulement ; le stockage est un port pour qu'un adaptateur distant s'ajoute plus tard sans toucher au domaine.

## 5. Architecture (clean architecture, Go)

Dépendances orientées vers l'intérieur uniquement.

```
domain/       entités et règles (Vault, Project, Environment, Secret, SecretName…), sans dépendance
app/          use cases (CreateVault, Unlock, GetSecret, RunWithEnv, CompareEnvs…) + ports (interfaces)
infra/        adaptateurs : sqlite, crypto (Argon2id, XChaCha20-Poly1305), keychain, clipboard, fs
interfaces/   cli (Cobra) et desktop (bindings Wails) ; appellent uniquement app/
cmd/          secrio (CLI) et secrio-desktop : composition des dépendances
```

- Génériques Go avec parcimonie : p. ex. un `Repository[T]` ou des helpers de collection là où ils suppriment une vraie répétition ; pas de génériques décoratifs.
- Ports en petites interfaces, définies côté `app`.
- La structure par fonctionnalité de `specs.txt` (§10.2) est remplacée par cette structure en couches.

## 6. Modèle cryptographique

- Dérivation : Argon2id depuis le mot de passe principal.
- Chiffrement : XChaCha20-Poly1305, nonce unique par opération, clés via `crypto/rand`.
- Hiérarchie de clés :

```
mot de passe ──Argon2id──▶ clé de déverrouillage
                              └─ déchiffre ▶ clé du coffre (aléatoire)
                                                └─ déchiffre ▶ clé d'env (une par environnement)
                                                                  └─ déchiffre ▶ noms et valeurs des secrets de l'env
```

- **Une clé par environnement** : permet plus tard un token CI limité à un seul env (staging ne lit pas prod).
- Changer le mot de passe réenveloppe uniquement la clé du coffre.
- Format cryptographique versionné ; toute donnée persistée porte sa version.
- Tout est chiffré, y compris les noms de secrets (option B). Les métadonnées strictement nécessaires au déverrouillage (paramètres Argon2id, versions, identifiants) restent en clair.

## 7. Comportements clés

- **Verrouillage** : manuel, à l'inactivité (délai configurable), à la fermeture, à la mise en veille si détectable.
- **Session CLI** : `secrio unlock` place une clé de session dans le keychain de l'OS avec TTL ; `lock` la supprime. Sans keychain ou en CI : mot de passe via variable/stdin.
- **Résolution du contexte CLI** : `.secrio.toml` à la racine du repo (`vault`, `project`, `env`), sans secret, versionnable. Priorité : flags > `.secrio.toml` > config globale. Sans contexte suffisant : erreur claire, jamais de menu interactif (CI déterministe).
- **`run`** : construit l'environnement du processus enfant (variables existantes préservées sauf surcharge explicite), lance la commande, propage le code de sortie, pas de fichier temporaire en clair.
- **Historique** : modifications importantes tracées, jamais en clair ; versions restaurables après authentification.
- **Clipboard** : effacement automatique configurable, sans écraser un contenu copié entre-temps.
- **Logs et erreurs** : jamais de valeurs, mots de passe, clés ni tokens.
- **Comparaison d'envs** : présence/absence et différences, jamais les valeurs.

## 8. Contraintes

- Aucun service externe requis pour le fonctionnement local.
- Binaire CLI sans CGO ; démarrage rapide ; poids contenu.
- Desktop : Wails + React/TypeScript/Vite/Tailwind/shadcn/Zustand (stack de `specs.txt`, inchangée).
- Concurrence desktop/CLI sur le même coffre (SQLite WAL).
- Go ne garantit pas l'effacement mémoire : minimiser les copies, nettoyer au mieux, documenter la limite.
- Tests : unitaires, intégration, sécurité (corruption, absence de clair dans SQLite, permissions fichiers, `go test -race`), multiplateforme.

## 9. Ordre de construction

1. **Core** : domain, app, infra (crypto, SQLite, keychain), tests crypto et sécurité.
2. **CLI** (lecture seule + `run`).
3. **Desktop** (gestion complète).
4. Stabilisation : historique poussé, sauvegardes, packaging, CI/CD, docs.

Chaque étape fera l'objet de son propre plan.

## 10. Laissé au builder

- Choix précis des paramètres Argon2id (coût mémoire/temps) selon benchmarks sur machines cibles.
- Schéma SQL exact, système de migrations, découpage fin des packages dans chaque couche.
- Bibliothèque keychain/clipboard par OS.
- Emplacement par défaut des données et de la config par OS.
- Format de la clé de session et de son TTL par défaut.
- Format du registre des coffres, schéma exact de l'historique.
- Documentation d'architecture à maintenir dans `docs/`.

## 11. Décisions complémentaires (après relecture)

Validées par l'utilisateur :
- **Session partagée** entre CLI et desktop : `secrio unlock` déverrouille aussi l'app et inversement, via une clé de session unique dans le keychain.
- **Noms de coffres en clair** dans un petit registre local, pour que `vault list` fonctionne coffre verrouillé. Un coffre = un fichier SQLite. Projets, envs, secrets et leurs noms restent chiffrés. Exception assumée à l'anti-goal « aucun nom lisible sur disque ».

Choix du rédacteur (à vérifier à la relecture) :
- **Volume visé** : centaines de secrets par env, milliers par coffre. Déchiffrement complet en mémoire, pas d'index aveugle.
- **Session** (amendé le 2026-11-19 après la relecture finale) : le keychain de l'OS ne contient qu'une clé de session aléatoire par session (32 octets) ; la clé du coffre est scellée sous cette clé dans la ligne `sessions` du fichier du coffre. `Lock` supprime la ligne ; l'expiration (TTL, inactivité) est paresseuse : rien n'est supprimé avant le prochain `Open`, `Lock` ou `Unlock`, donc après le TTL la ligne et l'élément de keychain persistent et qui détient les deux moitiés peut encore déchiffrer le blob hors ligne jusqu'au prochain `Open`/`Lock` suivant l'expiration. `expires_at` et `idle_ms` sont liés au blob (AAD) ; `last_used` reste en clair et non authentifié ; reculer l'horloge système prolonge une session. Un élément de keychain seul, ou un fichier de coffre seul, ne permet pas d'ouvrir le coffre ; limite assumée : un attaquant du même utilisateur qui lit les deux pendant une session active obtient la clé du coffre. Conseil : TTL courts et `Lock` à la sortie. Reste valide après changement de mot de passe (réenveloppe de la même clé). Une exécution `run` lit dans une transaction de lecture unique (instantané cohérent).
- **Verrouillage** (TTL de session et inactivité) géré par le Core via un port d'horloge.
- **Historique** : trace valeurs, noms, création/suppression de secrets et d'envs ; chiffré sous la clé d'env (supprimer un env supprime son historique) ; pas de limite de rétention en v1 ; restauration = ressaisie du mot de passe. La table d'historique existe dès l'étape 1 (pas de migration ultérieure) ; l'UI vient plus tard.
- **Mot de passe non interactif** : `SECRIO_PASSWORD` ou stdin pour les scripts. L'accès CI complet reste hors v1.
- **Formats d'import/export desktop** : `.env`, JSON, YAML, avec avertissement explicite sur tout export en clair.
- **`export` en CLI** : absent en v1 (desktop uniquement) ; le bundle chiffré pour la CI viendra avec l'étape CI.
