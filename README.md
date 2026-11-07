# loto-stats-webapp

Statistiques Loto / EuroMillions (FDJ) : génération de grilles, statistiques
(fréquences, écarts, sommes, parité, test χ²), vérification d'une grille sur tout
l'historique. Portage web du script `loto.py`.

Stack : **Go** (`net/http`, `html/template`) + **htmx**, SQLite pure Go
(`modernc.org/sqlite`, sans CGO). Un seul binaire, templates et assets embarqués.

## Démarrer

```bash
make run          # http://localhost:8080, auth désactivée
make test
```

## Principe des données

- Au démarrage, tous les `*.csv` de `DATA_DIR` (par défaut `./data`) sont importés
  dans une base SQLite **en mémoire**. Le jeu (Loto / EuroMillions) est détecté
  depuis l'en-tête, les doublons (même jeu, même date) sont ignorés.
- La base n'est qu'un cache : rien à sauvegarder. Un redémarrage recharge simplement
  les CSV.
- Pas d'import depuis l'interface : pour mettre à jour l'historique, ajouter un nouveau
  CSV dans `DATA_DIR` (en Kubernetes : via l'init job / volume) et redémarrer.
- Les grilles générées ne sont pas stockées.

## Configuration (variables d'environnement)

| Variable | Défaut | Rôle |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | adresse d'écoute |
| `DATA_DIR` | `./data` | CSV chargés au démarrage |
| `DB_PATH` | *(vide = mémoire)* | fichier SQLite, utile pour du debug |
| `AUTH_ENABLED` | `false` | active la connexion Google |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | | client OAuth « Application Web » |
| `BASE_URL` | | URL publique, ex. `https://loto.example.com` |
| `COOKIE_SECURE` | `true` si `BASE_URL` en https | cookies `Secure` |

## Authentification Google

- OpenID Connect avec Google ; **tout compte Google à l'e-mail vérifié** est accepté.
- Pas de session serveur : l'ID token Google (JWT signé) est stocké dans un cookie
  `HttpOnly` et vérifié à chaque requête (signature, audience, émetteur, expiration).
  La session dure donc exactement la vie du jeton (~1 h).
- Dans la console Google Cloud, créer un client OAuth « Application Web » avec l'URI
  de redirection `${BASE_URL}/auth/callback`.
- Routes publiques : `/login`, `/auth/*`, `/static/*`, `/healthz`, `/readyz`.

## Kubernetes (à venir)

- Image : `make docker` (distroless, non-root, `data/` inclus par défaut).
- Sondes : `GET /healthz` (liveness), `GET /readyz` (prêt une fois les CSV chargés).
- Arrêt propre sur `SIGTERM`.
- Données : monter un volume sur `DATA_DIR`, rempli par un init container.
- Chaque pod a sa propre base en mémoire, construite depuis les mêmes CSV : les
  répliques sont identiques et sans état partagé.

## Arborescence

```
cmd/loto-stats/      point d'entrée (config, bootstrap des CSV, serveur HTTP)
internal/config/     lecture des variables d'environnement
internal/lottery/    domaine : jeux, stats, génération, vérification (sans I/O)
internal/store/      SQLite + import CSV
internal/auth/       OIDC Google, middleware
internal/server/     routes, handlers, vues htmx
web/templates/       templates HTML (pages + fragments htmx)
web/static/          CSS (thèmes sombre / clair), JS minimal, htmx vendored
data/                historiques FDJ par défaut
```

## Thèmes

Sombre et clair. Par défaut, le thème suit le système ; le bouton dans l'en-tête
permet de forcer l'un ou l'autre (mémorisé dans le navigateur).
