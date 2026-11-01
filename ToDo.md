# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §5); research goes in docs/research/.

## 1.6 memory on a real graphics card is inferred
Calibrated in the VM up to 7.5 GiB texture payload (2026-11-01). Most of the VM's texture RAM is the driver's copy, which the VMware adapter (no dedicated VRAM) commits as process memory; the model now counts it as VRAM and adds back only what does not fit in 90% of the card (research: unity16-texture-memory-breakdown). The 720 list: 36–52 GiB resident without dedicated VRAM, 15–30 GiB with a 24 GiB card. Next: run 1.6 once on a PC with a discrete GPU and compare Task Manager's "Commit size" and "Dedicated GPU memory" with the estimate (vanilla: about 0.9 GiB commit, 0.66 GiB dedicated).

## Loading animation
Maybe add a UI loading animation--low priority for now

## Archo Agent
Archo should be a Mod development agent that can take action, not just translate or tag