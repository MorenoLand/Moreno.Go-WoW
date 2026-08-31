# UI parity backlog

- [ ] Separate ESRB notice and version text so neither overlaps at the reference resolution.
- [ ] Remove unintended gaps from live input-box textures while preserving their MPQ-sourced artwork.
- [ ] Finish edit-box behavior: click-to-caret, selection replacement, reliable navigation, deletion, and typing.
- [ ] Repair vertical line/gap artifacts in options and other framed menus.
- [ ] Keep dropdowns above their owning option dialogs and route their clicks to the visible menu.
- [ ] Replace character-list numeric zone/map leftovers with the live localized location value.
- [ ] Match the reference widths for Change Realm, Create New Character, and related buttons.
- [ ] Repair the red character/addon/enter-world/create/back button texture path and alpha handling.
- [ ] Make realm rows, Okay, Cancel, and Change Realm fully clickable without returning through Escape.
- [ ] Remove duplicate addon rows/lines and wire addon selection plus all dialog buttons.
- [ ] Complete character creation layout: wider Randomize, correct description panels, clean left panel, preview model, shadow, and animated background.
- [ ] Complete the main-menu scene: restore missing model pieces and dragon rendering, animate snow and scene effects, and load all live MPQ assets.
- [ ] Verify visual changes from the actual `go run .` window with OS-level captures; do not treat framebuffer color artifacts as client behavior.
- [ ] Resolve race-specific character preview model paths from the live MPQ set; do not assume every race has an `Interface\\Glues\\Models\\UI_<Race>\\UI_<Race>.m2` asset.
- [ ] Make live MPQ cinematics decode and advance through the video at the authored display position and scale instead of stopping on load or rendering off-center.

## Verification

- [ ] Run the Go test suite and live MPQ smoke flow after each milestone.
- [ ] Compare login, options, video, sound, realm, addon, character-select, and character-create screens with `G:\Development\Rust\Warcraft\Images`.
- [ ] Keep temporary captures and diagnostics outside the repository and never stage credentials or reference screenshots.
