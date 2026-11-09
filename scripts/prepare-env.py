#!/usr/bin/env python3
"""NFR-04/05: generate local credentials without terminal output or overwrites."""
import argparse
import os
import re
import secrets
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=Path(".env"))
    parser.add_argument("--project", default="vkr-local")
    parser.add_argument("--db-port", type=int, default=5432)
    parser.add_argument("--backend-port", type=int, default=8080)
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]{0,62}", args.project):
        parser.error("invalid compose project name")
    if not all(1 <= port <= 65535 for port in (args.db_port, args.backend_port)):
        parser.error("ports must be in 1..65535")
    if args.db_port == args.backend_port:
        parser.error("ports must differ")
    password = secrets.token_hex(32)
    authority = f"svoyshef:{password}"
    content = (
        f"COMPOSE_PROJECT_NAME={args.project}\n"
        "POSTGRES_USER=svoyshef\n"
        f"POSTGRES_PASSWORD={password}\n"
        "POSTGRES_DB=svoyshef\n"
        f"DATABASE_URL=postgres://{authority}@db:5432/svoyshef?sslmode=disable\n"
        f"LOCAL_DATABASE_URL=postgres://{authority}@127.0.0.1:{args.db_port}/svoyshef?sslmode=disable\n"
        f"M2_IT_DATABASE_URL=postgres://{authority}@127.0.0.1:{args.db_port}/m5_integration?sslmode=disable\n"
        f"DB_PORT={args.db_port}\nBACKEND_PORT={args.backend_port}\n"
        "COOKIE_MAX_AGE_H=12\nAVAILABILITY_TTL_S=5\n"
        f"PORT={args.backend_port}\nRESTAURANT=aidabaran\n"
        "DATA_DIR=../data\nFRONTEND_DIST=../frontend/dist\n"
    )
    try:
        fd = os.open(args.out, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w", encoding="ascii") as stream:
            stream.write(content)
    except OSError:
        parser.exit(1, "Cannot create env file (parent must exist; existing files are never replaced).\n")
    print("Private env created. Keep it outside version control; do not print its contents.")


if __name__ == "__main__":
    main()
