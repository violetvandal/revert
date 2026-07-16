#!/usr/bin/env bash
# gpu.sh — shared, read-only GPU diagnostics for Revert. Sourced by the `revert`
# dispatcher (doctor) and share/run/revert-run.sh (a launch heads-up).
#
# THUG2 renders through DXVK, which by default binds the highest-ranked Vulkan device —
# usually the DISCRETE GPU. On a box whose discrete GPU is weak or on a poor driver
# (nouveau can't reclock Kepler; llvmpipe is CPU software rendering) while a healthy
# integrated GPU exists, that default is exactly backwards and the game crawls. There is
# no SAFE automatic pick — we can't know which GPU the user's monitor is actually on — so
# we DETECT and ADVISE, and let `revert run --gpu <name>` / GPU_FILTER do the choosing
# (both feed DXVK_FILTER_DEVICE_NAME, a substring match on the adapter's deviceName).

# gpu_list — one "TYPE<TAB>DRIVER<TAB>NAME" line per Vulkan adapter, via vulkaninfo.
# Silent (and returns 0) if vulkaninfo is absent or lists nothing. vulkaninfo --summary
# prints, per device, deviceType then deviceName then driverName; we emit on driverName.
gpu_list() {
  command -v vulkaninfo >/dev/null 2>&1 || return 0
  vulkaninfo --summary 2>/dev/null | awk '
    /deviceType[[:space:]]*=/ { sub(/.*PHYSICAL_DEVICE_TYPE_/,""); t=$0 }
    /deviceName[[:space:]]*=/ { sub(/.*=[[:space:]]*/,""); n=$0 }
    /driverName[[:space:]]*=/ { sub(/.*=[[:space:]]*/,""); d=$0;
                                if (t!="") printf "%s\t%s\t%s\n", t, d, n;
                                t=""; n=""; d="" }'
}

# gpu_advise EMIT — inspect the adapters and print guidance via the EMIT function (e.g.
# doctor's `note`, run's `log`) only when DXVK's default pick is worth questioning:
#   * MORE THAN ONE hardware GPU — DXVK defaults to the discrete one, which may be the
#     wrong choice (e.g. a weak/nouveau discrete card next to a healthy iGPU);
#   * ZERO hardware GPUs — only a software rasterizer (llvmpipe) is visible, so DXVK would
#     render on the CPU (a slideshow).
# A software rasterizer merely EXISTING alongside a real GPU is normal on every Mesa
# desktop and never gets picked, so it is NOT flagged. Returns 0 if it emitted anything,
# 1 if it stayed silent (a single healthy GPU). Never blocks, never auto-overrides.
gpu_advise() {
  local emit="${1:-echo}"
  local lines; lines="$(gpu_list)"
  [[ -n "$lines" ]] || return 1
  local type drv name ngpu=0 nvnote=""
  local -a adapters=()
  while IFS=$'\t' read -r type drv name; do
    [[ -n "$type" ]] || continue
    case "$type" in
      CPU|OTHER) ;;                                   # software / non-GPU: not a real choice
      *) ngpu=$((ngpu+1)); adapters+=("${name} [${drv}]")
         case "$drv" in *nouveau*) nvnote="one uses the nouveau driver, which is very slow on older NVIDIA cards";; esac ;;
    esac
  done <<< "$lines"

  local active="${DXVK_FILTER_DEVICE_NAME:-${GPU_FILTER:-}}"

  # No hardware GPU at all — DXVK would fall back to CPU software rendering.
  if (( ngpu == 0 )); then
    "$emit" "No hardware GPU visible to Vulkan — DXVK would software-render (a slideshow)."
    "$emit" "Install your GPU's Vulkan driver (e.g. mesa-vulkan-drivers + the 32-bit variant)."
    return 0
  fi

  # A single GPU is the normal case: only speak up to confirm a pinned filter.
  if (( ngpu == 1 )); then
    [[ -n "$active" ]] && { "$emit" "GPU filter active — DXVK uses the adapter matching \"$active\"."; return 0; }
    return 1
  fi

  # Multiple hardware GPUs: DXVK defaults to the discrete one; help the user choose.
  if [[ -n "$active" ]]; then
    "$emit" "GPU filter active — of $ngpu GPUs, DXVK uses the one matching \"$active\"."
    return 0
  fi
  "$emit" "Multiple GPUs detected; DXVK defaults to the discrete one:"
  local a; for a in "${adapters[@]}"; do "$emit" "  - $a"; done
  [[ -n "$nvnote" ]] && "$emit" "Note: $nvnote."
  "$emit" "If the game runs slowly, pick a GPU: revert run <lane> --gpu <name> (e.g. --gpu Intel),"
  "$emit" "or set GPU_FILTER=<name> in revert.conf.local (match a bracketed name above)."
  return 0
}
