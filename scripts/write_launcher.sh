#!/usr/bin/env bash
# write_launcher.sh INSTALL_DIR APP_NAME SHORT_NAME
# Called by Makefile install-global / install-global-llm targets.
set -e

INSTALL_DIR="$1"
APP_NAME="$2"
SHORT_NAME="$3"

mkdir -p "$INSTALL_DIR"

cat > "$INSTALL_DIR/$SHORT_NAME" << 'LAUNCHER'
#!/usr/bin/env bash
# gork — Gorkbot global launcher
# Loads .env from ~/.config/gorkbot/ (preferred) or the same dir as this script.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$SCRIPT_DIR/gorkbot"
for _env in "$HOME/.config/gorkbot/.env" "$SCRIPT_DIR/.env"; do
  if [ -f "$_env" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$_env"
    set +a
    break
  fi
done
exec "$BINARY" "$@"
LAUNCHER

chmod +x "$INSTALL_DIR/$SHORT_NAME"

echo ""
echo "✅  Binary  : $INSTALL_DIR/$APP_NAME"
echo "✅  Launcher: $INSTALL_DIR/$SHORT_NAME"
echo ""
echo "Put your API keys in  ~/.config/gorkbot/.env  (or $INSTALL_DIR/.env)."
echo ""
echo "If ~/bin is not on PATH yet:"
echo "  echo 'export PATH=\"\$HOME/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
echo ""
echo "Then just type: gork"
