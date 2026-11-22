package archlint

// oversizedTable lists the functions over the size limits when the rule came in (17 §4
// "Function size"): file, function, then its complexity and lines at that time. Each may
// shrink but not grow, and its line goes once it is within the limits. Reason for every
// line: written before the rule; to be split. Never add a line. A plain table, not a Go
// map, so that deleting a line never realigns the others.
const oversizedTable = `
internal/app/credit.go (*App).CreditStep 53 159
internal/app/creditreplace.go (*App).replaceCreditLocked 74 252
internal/app/data.go (*App).UpdateDataProgress 41 128
internal/app/directories.go (*App).ManagedDirectories 47 80
internal/app/directories.go (*App).setDirectoryRecords 41 88
internal/app/envlinks.go (*App).prepareEnvLinks 69 198
internal/app/envs.go (*App).EnvAdd 48 139
internal/app/envs.go (*App).EnvSet 57 160
internal/app/gamedl.go errorKind 42 22
internal/app/linkhealth.go (*App).InspectGameLinks 61 172
internal/app/linkhealth.go (*App).applyLinkRepair 51 111
internal/app/linkrecovery.go (*App).applyLinkRecoveries 41 116
internal/app/pack.go (*App).BuildPackWith 41 135
internal/app/pack.go (*App).launchPackWith 64 233
internal/app/pack_archive.go (*App).exportPackArchive 50 161
internal/app/pack_archive.go scanPackArchive 44 118
internal/app/packsettings.go (*App).ApplyListSettings 52 166
internal/app/rimcrow.go (*App).ImportRimCrow 128 428
internal/app/rimcrowprofiles.go (*App).importCrowProfiles 80 291
internal/app/rimsortaux.go (*App).ImportRimSortAux 60 188
internal/app/scan.go (*App).scanEnvironment 58 224
internal/app/source.go (*App).SourceSearchContext 49 174
internal/app/subscribe.go (*App).UpdateSubscriptionsWith 61 240
internal/app/subscribeworkshop.go (*App).downloadWorkshopGroup 57 197
internal/app/uninstall.go (*App).planUninstall 150 467
internal/app/workshopsub.go (*App).Workshop 48 134
`
