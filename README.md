# todo

Kanban en terminal (Bubble Tea / Lip Gloss) dont les cartes peuvent exécuter des workflows [AWF](https://awf-project.ai).
Même famille que `awf-tui`.

```sh
make build
./todo -dir /chemin/du/projet-awf     # défaut : répertoire courant
./todo -awf /chemin/vers/awf          # binaire awf (défaut : config, puis "awf")
```

## Stockage

- **Board du projet** : `<dir>/.todo/board.yaml` (versionnable, écriture atomique).
- **Config globale** : `~/.config/todo/config.yaml` (`$XDG_CONFIG_HOME`), créée au premier lancement :
  binaire `awf` et colonnes données aux nouveaux boards.

```yaml
awf: awf
columns:
  - {name: Backlog}
  - {name: Doing, auto_run: true, on_success: Done, on_failure: Failed}
  - {name: Done}
  - {name: Failed}
```

`auto_run` : déplacer une carte dans la colonne lance son workflow. `on_success` / `on_failure` : colonne vers laquelle
la carte passe à la fin du run (selon la colonne où elle se trouve alors).

## Touches

| Touche | Action |
|---|---|
| ←→ `h` `l` · ↑↓ `j` `k` | naviguer |
| `H` `L` | déplacer la carte (colonne auto-run : lance le workflow) |
| `K` `J` | réordonner dans la colonne |
| `n` `e` `d` | nouvelle / éditer / supprimer (confirmation `y`) |
| `x` | exécuter le workflow de la carte (`awf run`) |
| `c` | annuler le run |
| `enter` | sortie du run (↑↓ défiler · `c` annuler · `r` relancer · `esc` retour ; le run continue) |
| `/` · `esc` | filtrer / effacer |
| `?` · `q` | aide / quitter (annule les runs en cours) |

Une carte : titre, description, workflow (liste issue de `.awf/workflows`, `~/.config/awf/workflows`, `~/.awf/workflows`)
et inputs `clé=valeur, …`. Badges : ⏵ en cours · ✓ · ✗ · ⊘ annulé.

## Limites

- La sortie d'un run n'est conservée qu'en mémoire (statut et id du dernier run sont, eux, enregistrés dans le board).
- Les steps `mode: conversation` sont gérés (invite `> ` dans la vue du run) ; pas le protocole `@@AWF-ASK@@` d'awf-tui.
