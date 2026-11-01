# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §5); research goes in docs/research/.

## Memory estimates for texture-heavy lists are extrapolated
Now calibrated up to 7.5 GiB texture payload (2026-11-01): above 3 GiB the coefficient drops to 1.3–1.6×. The 720 list (~24–27 GiB) estimates 36–52 GiB resident, still extrapolated ~3.5×. The guest pagefile could not be raised without a reboot. Next: a machine with ≥64 GB RAM, or a VM cold boot with a large pagefile (needs the user's Windows sign-in).

## Loading animation
Maybe add a UI loading animation--low priority for now

## Archo Agent
Archo should be a Mod development agent that can take action, not just translate or tag