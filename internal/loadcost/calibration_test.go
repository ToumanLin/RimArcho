package loadcost

import (
	"encoding/json"
	"testing"
)

// Numeric aggregate fixtures only: no game or Mod files. Source: 2026-10-31 VM
// private-byte calibration and 2026-11-01 heavy textures; see
// docs/research/2026-11-01_unity16-heavy-texture-memory.md.
func TestUnity16CalibrationCoverage(t *testing.T) {
	for _, tc := range []struct {
		name         string
		count        int
		aggregate    string
		stable, peak []int64
	}{
		{"vanilla", 2, `{"maxPngDecode":0,"png":{"count":0,"bytes":0},"pngPixels":0,"pngPixelsMip":0,"pngRaw16":{"count":0,"pixelsMip":0},"pngNot4":0,"pngSmall512Pixels":0,"dds":{"count":0,"bytes":0},"jpg":{"count":0,"bytes":0},"psd":{"count":0,"bytes":0},"defs":{"count":0,"bytes":0},"patches":{"count":0,"bytes":0},"languages":{"count":0,"bytes":0},"assemblies":{"count":3,"bytes":2543616}}`, []int64{1549029376}, []int64{2591825920}},
		{"light", 244, `{"maxPngDecode":27962368,"png":{"count":4005,"bytes":127594701},"pngPixels":619275960,"pngPixelsMip":826140160,"pngRaw16":{"count":5,"pixelsMip":1456},"pngNot4":65485408,"pngSmall512Pixels":87900387,"dds":{"count":7,"bytes":192592},"jpg":{"count":0,"bytes":0},"psd":{"count":0,"bytes":0},"sounds":{".ogg":{"count":296,"bytes":368141223,"small":{"count":218,"bytes":14467552},"smallPcm16Bytes":159143072,"resident":{"count":212,"bytes":13074060}},".wav":{"count":26,"bytes":9126116,"small":{"count":21,"bytes":2787318},"smallPcm16Bytes":2701200,"resident":{"count":25,"bytes":4824960}}},"defs":{"count":994,"bytes":6433610},"patches":{"count":334,"bytes":801516},"languages":{"count":1542,"bytes":4374477},"assemblies":{"count":278,"bytes":49066552}}`, []int64{4274323456}, []int64{5973250048}},
		{"large", 494, `{"maxPngDecode":27962368,"png":{"count":15255,"bytes":324444143},"pngPixels":1975286747,"pngPixelsMip":2634834624,"pngRaw16":{"count":61,"pixelsMip":18448},"pngNot4":72410000,"pngSmall512Pixels":544032565,"dds":{"count":10,"bytes":258208},"jpg":{"count":2,"bytes":223842},"psd":{"count":7,"bytes":593904},"sounds":{".ogg":{"count":477,"bytes":377802652,"small":{"count":394,"bytes":22142265},"smallPcm16Bytes":243564915,"resident":{"count":386,"bytes":20231064}},".mp3":{"count":2,"bytes":36825,"small":{"count":2,"bytes":36825},"smallPcm16Bytes":405075,"resident":{"count":2,"bytes":36825}},".wav":{"count":53,"bytes":15263722,"small":{"count":44,"bytes":5726430},"smallPcm16Bytes":5636626,"resident":{"count":52,"bytes":10962566}}},"defs":{"count":2499,"bytes":26514728},"patches":{"count":936,"bytes":2780791},"languages":{"count":2987,"bytes":7699060},"assemblies":{"count":527,"bytes":70847736}}`, []int64{8844439552}, []int64{11667345408}},
		{"texture", 244, `{"maxPngDecode":85402496,"png":{"count":4994,"bytes":630572670},"pngPixels":976207626,"pngPixelsMip":1302117808,"pngRaw16":{"count":5,"pixelsMip":1456},"pngNot4":95210048,"pngSmall512Pixels":116815034,"dds":{"count":1197,"bytes":351795688},"jpg":{"count":0,"bytes":0},"psd":{"count":0,"bytes":0},"sounds":{".ogg":{"count":361,"bytes":362868500,"small":{"count":289,"bytes":12517104},"smallPcm16Bytes":137688144,"resident":{"count":289,"bytes":12517104}},".wav":{"count":38,"bytes":16397808,"small":{"count":32,"bytes":4636406},"smallPcm16Bytes":4464442,"resident":{"count":36,"bytes":6674048}},".mp3":{"count":1,"bytes":18016,"small":{"count":1,"bytes":18016},"smallPcm16Bytes":198176,"resident":{"count":1,"bytes":18016}}},"defs":{"count":1163,"bytes":6082141},"patches":{"count":387,"bytes":2787452},"languages":{"count":1742,"bytes":5409281},"assemblies":{"count":277,"bytes":55478432}}`, []int64{5780566016, 5827825664}, []int64{7887273984, 7607316480}},
		{"h-4p5-menu", 244, `{"maxPngDecode":237212864,"png":{"count":12123,"bytes":950181413},"pngPixels":2719581527,"pngPixelsMip":3626980512,"pngRaw16":{"count":7,"pixelsMip":1504},"pngNot4":155464496,"pngSmall512Pixels":362615574,"dds":{"count":749,"bytes":276794680},"jpg":{"count":1,"bytes":377746},"psd":{"count":0,"bytes":0},"defs":{"count":2122,"bytes":15774410},"patches":{"count":530,"bytes":3273529},"languages":{"count":2941,"bytes":7299964},"assemblies":{"count":283,"bytes":62958048},"sounds":{".ogg":{"count":405,"bytes":384552133,"small":{"count":324,"bytes":14820686},"smallPcm16Bytes":163027546,"resident":{"count":322,"bytes":14339656}},".mp3":{"count":2,"bytes":36825,"small":{"count":2,"bytes":36825},"smallPcm16Bytes":405075,"resident":{"count":2,"bytes":36825}},".wav":{"count":40,"bytes":23291058,"small":{"count":32,"bytes":4636406},"smallPcm16Bytes":4464442,"resident":{"count":37,"bytes":7409152}}}}`, []int64{9681448960}, []int64{12838711296}},
		{"h-6-menu", 244, `{"maxPngDecode":237212864,"png":{"count":15838,"bytes":1176202680},"pngPixels":3436886495,"pngPixelsMip":4583579296,"pngRaw16":{"count":11,"pixelsMip":1568},"pngNot4":165608816,"pngSmall512Pixels":492201542,"dds":{"count":1273,"bytes":367071744},"jpg":{"count":1,"bytes":377746},"psd":{"count":0,"bytes":0},"defs":{"count":2169,"bytes":16547884},"patches":{"count":550,"bytes":3247460},"languages":{"count":3039,"bytes":7518840},"assemblies":{"count":279,"bytes":62816224},"sounds":{".ogg":{"count":405,"bytes":384552133,"small":{"count":324,"bytes":14820686},"smallPcm16Bytes":163027546,"resident":{"count":322,"bytes":14339656}},".mp3":{"count":3,"bytes":54841,"small":{"count":3,"bytes":54841},"smallPcm16Bytes":603251,"resident":{"count":3,"bytes":54841}},".wav":{"count":40,"bytes":23291058,"small":{"count":32,"bytes":4636406},"smallPcm16Bytes":4464442,"resident":{"count":37,"bytes":7409152}}}}`, []int64{10894417920}, []int64{14398750720}},
		{"h-8-full", 244, `{"maxPngDecode":237212864,"png":{"count":20084,"bytes":1526687761},"pngPixels":4708511611,"pngPixelsMip":6279684688,"pngRaw16":{"count":12,"pixelsMip":1584},"pngNot4":260667792,"pngSmall512Pixels":579864138,"dds":{"count":1787,"bytes":424106272},"jpg":{"count":3,"bytes":632708},"psd":{"count":0,"bytes":0},"defs":{"count":2674,"bytes":21668864},"patches":{"count":592,"bytes":3455573},"languages":{"count":3468,"bytes":9014607},"assemblies":{"count":284,"bytes":62795744},"sounds":{".ogg":{"count":396,"bytes":428786570,"small":{"count":300,"bytes":20675699},"smallPcm16Bytes":227432689,"resident":{"count":289,"bytes":18159821}},".mp3":{"count":34,"bytes":11860502,"small":{"count":30,"bytes":3580185},"smallPcm16Bytes":39382035,"resident":{"count":25,"bytes":2468031}},".wav":{"count":54,"bytes":29594260,"small":{"count":34,"bytes":4941186},"smallPcm16Bytes":4851184,"resident":{"count":51,"bytes":13712354}}}}`, []int64{13764616192, 14017335296}, []int64{17069928448, 17100443648}},
		{"h-baseline-menu", 244, `{"maxPngDecode":85402496,"png":{"count":4994,"bytes":630572670},"pngPixels":976207626,"pngPixelsMip":1302117808,"pngRaw16":{"count":5,"pixelsMip":1456},"pngNot4":95210048,"pngSmall512Pixels":116815034,"dds":{"count":1197,"bytes":351795688},"jpg":{"count":0,"bytes":0},"psd":{"count":0,"bytes":0},"defs":{"count":1163,"bytes":6082141},"patches":{"count":387,"bytes":2787452},"languages":{"count":1742,"bytes":5398913},"assemblies":{"count":276,"bytes":55407264},"sounds":{".ogg":{"count":361,"bytes":362868500,"small":{"count":289,"bytes":12517104},"smallPcm16Bytes":137688144,"resident":{"count":289,"bytes":12517104}},".wav":{"count":38,"bytes":16397808,"small":{"count":32,"bytes":4636406},"smallPcm16Bytes":4464442,"resident":{"count":36,"bytes":6674048}},".mp3":{"count":1,"bytes":18016,"small":{"count":1,"bytes":18016},"smallPcm16Bytes":198176,"resident":{"count":1,"bytes":18016}}}}`, []int64{5703548928}, []int64{7471529984}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mods := make([]Mod, tc.count)
			if err := json.Unmarshal([]byte(tc.aggregate), &mods[0]); err != nil {
				t.Fatal(err)
			}
			r := Estimate(Game16, mods)
			// The VMware adapter has no dedicated VRAM: every GPU byte is private bytes.
			ram, vram := Baseline(Game16)
			sLow, sHigh := Spill(Game16, r.VRAMLow+vram, r.VRAMHigh+vram, 0)
			r.RAMLow, r.RAMHigh = r.RAMLow+sLow, r.RAMHigh+sHigh
			base := ram
			lo, hi := LoadingBudget(Game16, r, mods)
			for _, v := range tc.stable {
				if v < r.RAMLow+base || v > r.RAMHigh+base {
					t.Fatalf("stable %d outside %d–%d", v, r.RAMLow+base, r.RAMHigh+base)
				}
			}
			for _, v := range tc.peak {
				if v < lo+base || v > hi+base {
					t.Fatalf("peak %d outside %d–%d", v, lo+base, hi+base)
				}
			}
			if r.RAMHigh+base > (r.RAMLow+base)*135/100 {
				t.Fatal("resident interval wider than calibration tolerance")
			}
			t.Logf("resident %.3f–%.3f GiB; startup %.3f–%.3f GiB", float64(r.RAMLow+base)/(1<<30), float64(r.RAMHigh+base)/(1<<30), float64(lo+base)/(1<<30), float64(hi+base)/(1<<30))
		})
	}
}
