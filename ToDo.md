# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §5); research goes in docs/research/.

## Memory estimates for texture-heavy lists are extrapolated
1.6 memory was calibrated against lists with 1–3 GiB of texture payload in the VM (2026-10-31, research Unity16 memory calibration). Lists with over 20 GiB of texture payload (such as the 720-Mod list) currently estimate 43–57 GiB resident and 47–71 GiB during loading, about 8 times beyond the calibration range; they could not load in the 8 GiB VM, so actual usage could not be measured. Measure once on a machine with enough memory, then decide whether the texture coefficient stays linear at the high end

## Localization
docs should be in English, with no Chinese in the project except localization and the glossary;
a Chinese ReadMe is needed

## Loading animation
Maybe add a UI loading animation--low priority for now

## Archo Agent
Archo should be a Mod development agent that can take action, not just translate or tag