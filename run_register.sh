#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="${ROOT_DIR}/.venv"
PY="${VENV_DIR}/bin/python"

if [ ! -x "${PY}" ]; then
  python3 -m venv "${VENV_DIR}"
fi

if ! "${PY}" -m pip show curl_cffi >/dev/null 2>&1; then
  "${PY}" -m pip install curl_cffi
fi
if ! "${PY}" -m pip show pybase64 >/dev/null 2>&1; then
  "${PY}" -m pip install pybase64
fi

exec "${PY}" "${ROOT_DIR}/chatgpt_协议注册机.py" "$@"
