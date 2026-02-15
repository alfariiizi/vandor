# Installation Guide

This guide covers Vandor installation for Linux, macOS, Windows (native), and WSL.

## Linux / macOS / WSL

Install latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/alfariiizi/vandor/main/scripts/install.sh | sh
```

Install specific version:

```bash
VANDOR_VERSION=v0.4.0 curl -fsSL https://raw.githubusercontent.com/alfariiizi/vandor/main/scripts/install.sh | sh
```

Optional custom install directory:

```bash
VANDOR_INSTALL_DIR="$HOME/.local/bin" curl -fsSL https://raw.githubusercontent.com/alfariiizi/vandor/main/scripts/install.sh | sh
```

After install:

```bash
vandor --version
```

Optional official registry override:

```bash
export VANDOR_VPKG_REGISTRY_OFFICIAL=https://vpkg.vercel.app
```

## Windows (native)

### Manual install (recommended)

1. Download `vandor_<version>_windows_amd64.zip` from GitHub Releases.
2. Extract `vandor.exe` to a stable folder, for example:
   - `C:\Tools\vandor\`
3. Add that folder to User PATH manually.
4. Open a new PowerShell window and run:

```powershell
vandor --version
```

### PowerShell script install

Download and run:

```powershell
iwr https://raw.githubusercontent.com/alfariiizi/vandor/main/scripts/install.ps1 -OutFile install.ps1
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

Optional:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1 -AddToPath
setx VANDOR_VPKG_REGISTRY_OFFICIAL "https://vpkg.vercel.app"
```

## Verify VPKG Registry

```bash
vandor vpkg registry add official https://vpkg.vercel.app
vandor vpkg search storage --registry official
```
