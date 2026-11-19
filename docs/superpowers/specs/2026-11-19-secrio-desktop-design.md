# Secrio Desktop — Design

Date : 2026-11-19
Statut : brouillon, en attente d'approbation
Base : `2026-11-19-secrio-design.md` (§2 rôles, §5 Desktop), le Core et la CLI déjà dans `main`.

## 1. Intention et pourquoi

Une application desktop Wails pour **écrire et gérer** : insérer facilement des secrets, gérer ses projets et ses envs, importer un `.env`. La CLI reste l'outil de lecture et d'exécution. Les deux partagent le même Core, les mêmes fichiers de coffre et la même session (clé de session dans le keychain, ligne `sessions` dans le coffre).

Modèle visuel (donné par l'utilisateur) : un gestionnaire de mots de passe sombre type « Passary » : barre du haut, sidebar, tableau, panneau de détail à droite. On arrive sur un **sélecteur de projet**, puis la **vue des envs avec les valeurs** (masquées par défaut), un env à la fois (variante A de la maquette, choisie par l'utilisateur).

Critères prioritaires (utilisateur) : **ouverture rapide**, **faible consommation**, sobriété de dépendances.

## 2. Périmètre du premier jet (découpe validée)

**Dans le premier jet**
- Écran de déverrouillage (choix du coffre, mot de passe) et **création d'un coffre** si aucun n'existe (nom, mot de passe, confirmation).
- Sélecteur de projet (cartes), création de projet, suppression de projet (confirmation).
- Vue d'un projet : sidebar des envs (nombre de secrets), création et suppression d'env (confirmation), tableau des secrets de l'env choisi : nom, valeur masquée, type, date de modification, actions (voir, copier, éditer, supprimer) ; recherche ; panneau de détail à droite (valeur avec œil et copier, type, dates).
- Création et édition de secret (nom, valeur, type), suppression (confirmation).
- Import d'un fichier `.env` (sélecteur de fichier, aperçu des nouveaux/doublons, stratégie ignorer/écraser).
- Copier avec effacement automatique du presse-papiers (30 s), sans écraser un contenu copié entre-temps.
- Verrouiller ; verrouillage à la fermeture de l'app.

**Reporté (non-goals du premier jet)** : comparaison d'envs, historique et restauration, réglages (délai de verrouillage, thème), export, palette Ctrl K, renommage de coffre/env/secret, suppression de coffre, favoris, tags, force du mot de passe, générateur, bouton « Enregistrer » (tout est enregistré immédiatement), i18n (anglais d'abord, structure prévue).

**Anti-goals** : une valeur de secret dans une liste, un log, une erreur ou le DOM d'un tableau ; du contenu distant dans la webview ; une dépendance lourde pour gagner un détail visuel ; un handle de coffre gardé ouvert entre deux actions.

## 3. Approches considérées (choix de l'utilisateur ou « tout me va »)

- **Wails** (choix de l'utilisateur) ; version : **v2 par défaut** (mon choix) ; v3 est annoncée stable dans son README (`v3.0.0`), plus récente, API différente. Un **spike de mesure** en première tâche du plan tranche (voir §8).
- **Langage du front** : React + TypeScript + Vite + Tailwind (choix de l'utilisateur, critère rapide/léger), sans Zustand ni routeur : état React local + un petit store maison si besoin. React plutôt que Svelte : le kit Better Design (shadcn, Base UI) n'existe que pour React. TypeScript parce que Wails génère les types depuis les méthodes Go.
- **Lien front ↔ Core** : une **façade Go** (`internal/desktop`) qui ouvre, agit, referme à chaque appel et renvoie des DTO ; écarté : handle gardé en mémoire (ignore `Lock`/expiration), pilotage du binaire CLI.
- **Verrouillage à la fermeture** : oui (recommandation acceptée) ; conséquence assumée : la session partagée est aussi fermée pour la CLI ; configurable plus tard.

## 4. Architecture

```
cmd/secrio-desktop/main.go        point d'entrée Wails minimal (options, OnShutdown)
internal/desktop/                 façade Go liée au front (couche « interfaces », comme internal/cli)
desktop/frontend/                 React + TypeScript + Vite + Tailwind (+ composants du design system)
```
- `internal/desktop` importe `internal/core`, `internal/app`, `internal/domain` ; **jamais `internal/infra`** (même garde que la CLI).
- Chaque méthode liée : charge le contexte, `svc.Open(vaultID)` (ou `ErrLocked` → le front montre l'écran de déverrouillage), exécute une action du Core, `Close()`, renvoie un DTO. Aucun état de coffre dans la façade, à part le coffre « courant » choisi.
- DTO sans valeur : `SecretDTO{Name, Type, Description?, CreatedAt, UpdatedAt}` ; la valeur n'existe que dans la réponse de `RevealSecret`. Les DTO, les erreurs et les logs ne contiennent jamais de valeur ni de mot de passe.
- **Copier** : `CopySecret(...)` écrit dans le presse-papiers **côté Go** (la valeur ne passe jamais par le JS) et programme l'effacement après 30 s uniquement si le presse-papiers contient encore cette valeur. Si l'API Wails ne permet pas de lire le presse-papiers, ne pas effacer et le dire dans l'interface (voir §8).
- **Afficher** : seule `RevealSecret` renvoie la valeur ; le front la garde en état de composant au plus quelques secondes puis la masque de nouveau et la remplace.
- **Verrouillage** : `LockVault()` → `svc.Lock`. Toute opération qui renvoie `ErrLocked` (session expirée, verrouillage par la CLI) ramène à l'écran de déverrouillage. La fermeture de la fenêtre appelle `Lock` (hook d'arrêt Wails). Durées de session : mêmes défauts que la CLI (12 h, 1 h d'inactivité).
- Réglage minimal hors secret : le dernier coffre utilisé, dans un petit fichier JSON non sensible du dossier de données (`desktop.json`, 0600).
- **Ajouts au Core nécessaires** : `SecretCount(ctx, envID) (int, error)` sur `Unlocked` (compte les enregistrements sans déchiffrer ; sinon l'écran de projets devrait déchiffrer toutes les valeurs pour afficher un nombre).

## 5. Écrans (maquette A validée)

1. **Déverrouillage / premier lancement** : liste de coffres (ou création), mot de passe, bouton Déverrouiller.
2. **Sélecteur de projet** : cartes (nom, nombre de secrets, envs), « + Nouveau projet », recherche, Verrouiller en haut.
3. **Projet** : sidebar d'envs (nombre de secrets, « + env »), tableau (nom, valeur masquée, type, modifié, actions), recherche, « + Nouveau secret », « Importer .env », panneau de détail à droite.
4. **Dialogues** : nouveau projet, nouvel env, nouveau/éditer secret, import (aperçu), confirmations de suppression.

L'interface suit le workflow Better Design (voir §7).

## 6. Sécurité de la webview

Ressources locales embarquées uniquement, aucune requête réseau, politique de contenu stricte, pas d'outils de développement dans la version livrée, pas d'`eval`, pas de HTML injecté depuis des données. Les valeurs révélées ne sont jamais écrites dans le stockage du navigateur ni dans l'URL.

## 7. Conception visuelle et qualité

- Workflow Better Design de l'utilisateur : design system d'abord (choisir ou générer), principes UI/UX avant de coder, composants du kit installés dans le projet, puis relecture (règles, compréhension, espacements) avec correction des problèmes critiques et sérieux. Thème sombre.
- Tests : façade Go testée avec un vrai Core dans un dossier temporaire (clavier de session en mémoire, horloge factice) ; composants front testés avec Vitest + Testing Library pour les parcours clés ; une liste de vérification manuelle (démarrage, mémoire, verrouillage à la fermeture, copie/effacement).
- **Mesures à publier** : temps de démarrage à froid et mémoire au repos du binaire livré, sur la machine de développement.

## 8. Laissé au builder (et risques à lever par un spike)

- **Spike initial (Wails v2 et v3)** : fenêtre vide avec React, mesurer démarrage et mémoire, vérifier : lecture/écriture du presse-papiers, hook d'arrêt/fermeture, génération des types TypeScript, sélecteur de fichier, compilation avec Go 1.26 et CGO sur macOS. Choisir la version sur ces résultats (v2 par défaut).
- Disposition des fichiers de `internal/desktop`, nommage des méthodes liées, structure du front, choix du design system (Better Design), gestion du presse-papiers selon l'API disponible.
- Packaging (`.app` macOS d'abord ; signature/notarisation, Windows, Linux plus tard ; CGO requis pour la webview sur macOS/Linux).
- Messages d'erreur utilisateur et états vides.

## 9. Décisions prises par le rédacteur (à vérifier)

Wails v2 par défaut avec spike ; pas de Zustand ni routeur ; dernier coffre dans `desktop.json` ; durées de session 12 h/1 h ; création de coffre au premier lancement incluse ; suppression de projet/env avec confirmation incluse ; ajout de `SecretCount` au Core ; copie côté Go avec effacement conditionnel ; pas de renommage dans le premier jet.

Ajouts après la relecture du design (choix du rédacteur, non bloquants) :
- **Import `.env` sans état dans la façade** : la façade ne garde pas de plan en mémoire. L'aperçu (`PreviewImport(path)`) lit et analyse le fichier côté Go et renvoie seulement les noms nouveaux et en doublon. L'application (`ApplyImport(path, stratégie)`) **relit** le fichier, refait le plan, et **refuse** si les ensembles nouveaux/doublons diffèrent de ce que l'utilisateur a vu (le fichier ou le coffre a changé). Les valeurs ne passent jamais par le JS.
- **Presse-papiers** : l'effacement conditionnel (le presse-papiers contient encore notre valeur) a lieu aussi **à la fermeture de l'app et au verrouillage explicite**, pas seulement après 30 s. Si la lecture du presse-papiers n'est pas possible : ne pas effacer et l'indiquer dans l'interface.
- **Types et description** : le formulaire propose les **huit types** du Core (`text`, `password`, `api_key`, `token`, `url`, `private_key`, `certificate`, `json`), `text` par défaut ; les imports `.env` sont toujours `text`. La **description** reste dans les DTO mais n'est ni affichée ni éditable dans le premier jet.
