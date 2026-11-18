# todo

[English](README.md) · **Français**

Kanban en terminal (Bubble Tea / Lip Gloss) dont les cartes peuvent exécuter des workflows [AWF](https://awf-project.ai).
Même famille que `awf-tui`.

```sh
make build
./todo -dir /chemin/du/projet-awf     # défaut : répertoire courant
./todo -awf /chemin/vers/awf          # binaire awf (défaut : config, puis "awf")
./todo -transparent                   # garde le fond du terminal (ou `transparent: true` dans la config)
./todo -lang fr                       # langue de l'interface : en ou fr (défaut : langue du système, sinon en)
```

La langue est détectée via `LC_ALL`, `LC_MESSAGES`, `LANGUAGE` puis `LANG` ; `-lang` la force. Elle détermine aussi
le nom des colonnes créées par défaut (les boards et configs existants gardent les leurs).

## Stockage

- **Todos du projet** : `<dir>/.todo/board.yaml` (premier todo) et `<dir>/.todo/<nom>.yaml` pour les suivants (versionnables, écriture atomique). Chaque todo est un board complet ; les workflows d'un todo continuent quand on en affiche un autre.
- **Config globale** : `~/.config/todo/config.yaml` (`$XDG_CONFIG_HOME`), créée au premier lancement :
  binaire `awf` et colonnes données aux nouveaux boards.

```yaml
awf: awf
columns:
  - {name: Backlog, color: "#79c0ff"}
  - {name: À faire, color: "#f2cc60"}
  - {name: En cours (AWF), color: "#ff7ac6", auto_run: true, on_success: Terminé, on_failure: Terminé}
  - {name: Terminé, color: "#7ee787"}
```

`auto_run` marque la **colonne d'exécution** : `↵` y déplace la carte avant de lancer son workflow, et une carte dont la
case « Lancer automatiquement » est cochée s'y lance dès qu'elle y entre. `on_success` / `on_failure` : colonne vers
laquelle la carte passe à la fin du run (l'échec se lit au `✗` sur la carte). Les boards existants gardent leurs colonnes.

## Touches

| Touche | Action |
|---|---|
| ← ↑ ↓ → | naviguer |
| `↵` | lancer le workflow (la carte passe en colonne d'exécution) ; sur un run en cours : ouvre les logs |
| `H` `L` | déplacer la carte (carte « auto » + colonne d'exécution : lance le workflow) |
| `K` `J` | réordonner dans la colonne |
| `n` `e` `d` | nouvelle / éditer / supprimer (confirmation `y`) |
| `l` | logs du run (étapes, sortie horodatée) |
| `r` | relancer · `c` annuler |
| `tab` `maj+tab` | todo suivant / précédent (onglets en haut) |
| `N` `R` `D` | nouveau / renommer / supprimer le todo (confirmation `y`, jamais le dernier) |
| `/` · `esc` | rechercher (titre, workflow ; les cartes non trouvées sont atténuées) / effacer |
| `?` · `q` | aide / quitter (annule les runs en cours) |

Vue logs : `p` pause/reprise (SIGSTOP/SIGCONT), `c` annuler, `↑↓` défiler, `f` suivre la fin, `r` relancer, `esc` retour
(le run continue). Les étapes et la progression `étape k/n · %` viennent des `states:` du YAML du workflow.

Formulaire : titre, colonne, workflow (liste filtrable avec `/`, résumé des étapes), entrées `clé=valeur, …` facultatives,
case de lancement automatique. Badges : spinner en cours · ✓ · ✗ · ⊘ annulé.

## Limites

- La sortie d'un run n'est conservée qu'en mémoire (statut, id et durée du dernier run sont enregistrés dans le board).
- Les steps `mode: conversation` sont gérés (invite `> ` dans la vue des logs) ; pas le protocole `@@AWF-ASK@@` d'awf-tui.
- Le total d'étapes est déduit du YAML : il peut différer avec des boucles ou des branches.
