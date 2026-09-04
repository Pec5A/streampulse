#!/usr/bin/env python3
"""Diffusion de demonstration contre le backend deploye.

Publie en **WebSocket**, pas en POST chunked : le proxy de Render bufferise le
corps des requetes, donc une publication HTTP n'atteint l'application qu'une
fois l'envoi termine — le flux ne passe jamais "en direct" pendant qu'on
diffuse, et les auditeurs recoivent 404. Le WebSocket traverse le proxy sans
mise en tampon.

Vise le **local** par defaut : le script cree un compte et un flux a chaque
execution, et les laisser s'accumuler dans la base de production pollue la
liste des flux que l'equipe utilise pour ses propres essais.

    ./scripts/diffuser.py [duree_en_secondes]           # local
    API=https://streampulse-api-j46j.onrender.com \
      ./scripts/diffuser.py                            # production, explicite
"""
import asyncio, json, os, sys, threading, time, urllib.request, urllib.error

API = os.environ.get("API", "http://localhost:8080")
WS = API.replace("https://", "wss://").replace("http://", "ws://")
DUREE = int(sys.argv[1]) if len(sys.argv) > 1 else 120
DEBIT = int(os.environ.get("DEBIT", 16000))
AUDITEURS = int(os.environ.get("AUDITEURS", 2))


def api(path, data=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    body = json.dumps(data).encode() if data is not None else None
    return json.load(urllib.request.urlopen(urllib.request.Request(API + path, body, headers)))


def auditeur(numero, sid, recu):
    """Un auditeur : consomme le flux et compte ce qu'il recoit."""
    try:
        with urllib.request.urlopen(API + f"/api/v1/streams/{sid}/listen", timeout=DUREE) as r:
            while True:
                bloc = r.read(4096)
                if not bloc:
                    return
                recu[numero] += len(bloc)
    except urllib.error.HTTPError as e:
        print(f"  auditeur {numero} : refuse ({e.code})", flush=True)
    except Exception:
        return


async def main():
    import websockets

    n = int(time.time())
    token = api("/api/v1/auth/register",
                {"email": f"demo{n}@streampulse.test", "username": f"demo{n}",
                 "password": "MotDePasse123!"})["token"]
    sid = api("/api/v1/streams", {"title": "Demo soutenance", "description": "live"}, token)["id"]
    print(f"cible : {API}\nflux  : {sid}", flush=True)
    if "localhost" not in API and "127.0.0.1" not in API:
        print("  /!\\  cible de production : ce flux et ce compte y restent", flush=True)

    paquet = DEBIT // 10
    recu = [0] * AUDITEURS
    async with websockets.connect(f"{WS}/api/v1/streams/{sid}/publish/ws?token={token}",
                                  max_size=None) as ws:
        await ws.send(b"\0" * paquet)
        # Les auditeurs n'arrivent qu'une fois le hub reellement ouvert : se fier
        # au statut en base ne suffit pas, il est ecrit avant que le hub existe.
        for _ in range(40):
            await asyncio.sleep(0.25)
            if api(f"/api/v1/streams/{sid}")["status"] == "live":
                break
        print("etat  : a l'antenne", flush=True)

        for i in range(AUDITEURS):
            threading.Thread(target=auditeur, args=(i, sid, recu), daemon=True).start()
        print(f"auditeurs : {AUDITEURS} attaches", flush=True)
        print(f"\nDashboard : http://localhost:3000/d/streampulse-streaming\n", flush=True)

        debut = time.monotonic()
        while time.monotonic() - debut < DUREE:
            await ws.send(b"\0" * paquet)
            await asyncio.sleep(0.1)

    print("diffusion terminee — recu par auditeur :", recu, flush=True)


asyncio.run(main())
