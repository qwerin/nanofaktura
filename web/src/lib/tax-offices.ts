// Číselníky finančních úřadů pro podání přes EPO (DPHDP3, DPHKH1): `c_ufo` a `c_pracufo`.
//
// Zdroj: číselník „Územní finanční orgány“ (ufo) a „Územní pracoviště“ (pracufo) Generálního
// finančního ředitelství platný od 1. 1. 2013, převzato z přepisu
// https://pavlasoft.com/kontrolni-hlaseni/ciselnik-financnich-uradu.htm (ověřeno 09/2026) a
// podpory Finanční správy („Informace k číselníku ÚFO platném od 1.1.2013“):
// c_pracufo je nepovinné, rozhoduje c_ufo; Specializovaný FÚ má vždy c_ufo 13 a c_pracufo 4000.
// Pokud GFŘ číselník změní, stačí upravit tabulky níže.

export interface TaxOffice {
  code: string
  name: string
}

export interface TaxOfficeBranch {
  code: string
  office: string
  name: string
}

/** 14 krajských finančních úřadů + Specializovaný finanční úřad. */
export const TAX_OFFICES: readonly TaxOffice[] = [
  { code: '451', name: 'FÚ pro hlavní město Prahu' },
  { code: '452', name: 'FÚ pro Středočeský kraj' },
  { code: '453', name: 'FÚ pro Jihočeský kraj' },
  { code: '454', name: 'FÚ pro Plzeňský kraj' },
  { code: '455', name: 'FÚ pro Karlovarský kraj' },
  { code: '456', name: 'FÚ pro Ústecký kraj' },
  { code: '457', name: 'FÚ pro Liberecký kraj' },
  { code: '458', name: 'FÚ pro Královéhradecký kraj' },
  { code: '459', name: 'FÚ pro Pardubický kraj' },
  { code: '460', name: 'FÚ pro Kraj Vysočina' },
  { code: '461', name: 'FÚ pro Jihomoravský kraj' },
  { code: '462', name: 'FÚ pro Olomoucký kraj' },
  { code: '463', name: 'FÚ pro Moravskoslezský kraj' },
  { code: '464', name: 'FÚ pro Zlínský kraj' },
  { code: '13', name: 'Specializovaný finanční úřad' },
]

// [kód pracoviště, název] seskupené podle c_ufo
const BRANCHES: Record<string, [string, string][]> = {
  '451': [
    ['2001', 'Praha 1'], ['2002', 'Praha 2'], ['2003', 'Praha 3'], ['2004', 'Praha 4'], ['2005', 'Praha 5'],
    ['2006', 'Praha 6'], ['2007', 'Praha 7'], ['2008', 'Praha 8'], ['2009', 'Praha 9'], ['2010', 'Praha 10'],
    ['2011', 'Praha-Jižní Město'], ['2012', 'Praha-Modřany'],
  ],
  '452': [
    ['2101', 'Praha-východ'], ['2102', 'Praha-západ'], ['2103', 'Benešov'], ['2104', 'Beroun'],
    ['2105', 'Brandýs nad Labem-Stará Boleslav'], ['2106', 'Čáslav'], ['2107', 'Český Brod'], ['2108', 'Dobříš'],
    ['2109', 'Hořovice'], ['2110', 'Kladno'], ['2111', 'Kolín'], ['2112', 'Kralupy nad Vltavou'],
    ['2113', 'Kutná Hora'], ['2114', 'Mělník'], ['2115', 'Mladá Boleslav'], ['2116', 'Mnichovo Hradiště'],
    ['2117', 'Neratovice'], ['2118', 'Nymburk'], ['2119', 'Poděbrady'], ['2120', 'Příbram'], ['2121', 'Rakovník'],
    ['2122', 'Říčany'], ['2123', 'Sedlčany'], ['2124', 'Slaný'], ['2125', 'Vlašim'], ['2126', 'Votice'],
  ],
  '453': [
    ['2201', 'České Budějovice'], ['2202', 'Blatná'], ['2203', 'Český Krumlov'], ['2204', 'Dačice'],
    ['2205', 'Jindřichův Hradec'], ['2206', 'Kaplice'], ['2207', 'Milevsko'], ['2208', 'Písek'],
    ['2209', 'Prachatice'], ['2210', 'Soběslav'], ['2211', 'Strakonice'], ['2212', 'Tábor'],
    ['2213', 'Trhové Sviny'], ['2214', 'Třeboň'], ['2215', 'Týn nad Vltavou'], ['2216', 'Vimperk'], ['2217', 'Vodňany'],
  ],
  '454': [
    ['2301', 'Plzeň'], ['2302', 'Plzeň-sever'], ['2303', 'Plzeň-jih'], ['2304', 'Blovice'], ['2305', 'Domažlice'],
    ['2306', 'Horažďovice'], ['2307', 'Horšovský Týn'], ['2308', 'Klatovy'], ['2309', 'Kralovice'],
    ['2310', 'Nepomuk'], ['2311', 'Přeštice'], ['2312', 'Rokycany'], ['2313', 'Tachov'], ['2314', 'Stříbro'],
    ['2315', 'Sušice'],
  ],
  '455': [
    ['2401', 'Karlovy Vary'], ['2402', 'Aš'], ['2403', 'Cheb'], ['2404', 'Kraslice'], ['2405', 'Mariánské Lázně'],
    ['2406', 'Ostrov'], ['2407', 'Sokolov'],
  ],
  '456': [
    ['2501', 'Ústí nad Labem'], ['2502', 'Bílina'], ['2503', 'Děčín'], ['2504', 'Chomutov'], ['2505', 'Kadaň'],
    ['2506', 'Libochovice'], ['2507', 'Litoměřice'], ['2508', 'Litvínov'], ['2509', 'Louny'], ['2510', 'Most'],
    ['2511', 'Podbořany'], ['2512', 'Roudnice nad Labem'], ['2513', 'Rumburk'], ['2514', 'Teplice'], ['2515', 'Žatec'],
  ],
  '457': [
    ['2601', 'Liberec'], ['2602', 'Česká Lípa'], ['2603', 'Frýdlant'], ['2604', 'Jablonec nad Nisou'],
    ['2605', 'Jilemnice'], ['2606', 'Nový Bor'], ['2607', 'Semily'], ['2608', 'Tanvald'], ['2609', 'Turnov'],
    ['2610', 'Železný Brod'],
  ],
  '458': [
    ['2701', 'Hradec Králové'], ['2702', 'Broumov'], ['2703', 'Dobruška'], ['2704', 'Dvůr Králové nad Labem'],
    ['2705', 'Hořice'], ['2706', 'Jaroměř'], ['2707', 'Jičín'], ['2708', 'Kostelec nad Orlicí'], ['2709', 'Náchod'],
    ['2710', 'Nová Paka'], ['2711', 'Nový Bydžov'], ['2712', 'Rychnov nad Kněžnou'], ['2713', 'Trutnov'],
    ['2714', 'Vrchlabí'],
  ],
  '459': [
    ['2801', 'Pardubice'], ['2802', 'Hlinsko'], ['2803', 'Holice'], ['2804', 'Chrudim'], ['2805', 'Litomyšl'],
    ['2806', 'Moravská Třebová'], ['2807', 'Přelouč'], ['2808', 'Svitavy'], ['2809', 'Ústí nad Orlicí'],
    ['2810', 'Vysoké Mýto'], ['2811', 'Žamberk'],
  ],
  '460': [
    ['2901', 'Jihlava'], ['2902', 'Bystřice nad Pernštejnem'], ['2903', 'Havlíčkův Brod'], ['2904', 'Humpolec'],
    ['2905', 'Chotěboř'], ['2906', 'Ledeč nad Sázavou'], ['2907', 'Moravské Budějovice'],
    ['2908', 'Náměšť nad Oslavou'], ['2909', 'Pacov'], ['2910', 'Pelhřimov'], ['2911', 'Telč'], ['2912', 'Třebíč'],
    ['2913', 'Velké Meziříčí'], ['2914', 'Žďár nad Sázavou'],
  ],
  '461': [
    ['3001', 'Brno I'], ['3002', 'Brno II'], ['3003', 'Brno III'], ['3004', 'Brno IV'], ['3005', 'Brno-venkov'],
    ['3006', 'Blansko'], ['3007', 'Boskovice'], ['3008', 'Břeclav'], ['3009', 'Bučovice'], ['3010', 'Hodonín'],
    ['3011', 'Hustopeče'], ['3012', 'Ivančice'], ['3013', 'Kyjov'], ['3014', 'Mikulov'], ['3015', 'Moravský Krumlov'],
    ['3016', 'Slavkov u Brna'], ['3017', 'Tišnov'], ['3018', 'Veselí nad Moravou'], ['3019', 'Vyškov'], ['3020', 'Znojmo'],
  ],
  '462': [
    ['3101', 'Olomouc'], ['3102', 'Hranice'], ['3103', 'Jeseník'], ['3104', 'Konice'], ['3105', 'Litovel'],
    ['3106', 'Prostějov'], ['3107', 'Přerov'], ['3108', 'Šternberk'], ['3109', 'Šumperk'], ['3110', 'Zábřeh'],
  ],
  '463': [
    ['3201', 'Ostrava I'], ['3202', 'Ostrava II'], ['3203', 'Ostrava III'], ['3204', 'Bohumín'], ['3205', 'Bruntál'],
    ['3206', 'Český Těšín'], ['3207', 'Frýdek-Místek'], ['3208', 'Frýdlant nad Ostravicí'], ['3209', 'Fulnek'],
    ['3210', 'Havířov'], ['3211', 'Hlučín'], ['3212', 'Karviná'], ['3213', 'Kopřivnice'], ['3214', 'Krnov'],
    ['3215', 'Nový Jičín'], ['3216', 'Opava'], ['3217', 'Orlová'], ['3218', 'Třinec'],
  ],
  '464': [
    ['3301', 'Zlín'], ['3302', 'Bystřice pod Hostýnem'], ['3303', 'Holešov'], ['3304', 'Kroměříž'],
    ['3305', 'Luhačovice'], ['3306', 'Otrokovice'], ['3307', 'Rožnov pod Radhoštěm'], ['3308', 'Uherský Brod'],
    ['3309', 'Uherské Hradiště'], ['3310', 'Valašské Meziříčí'], ['3311', 'Valašské Klobouky'], ['3312', 'Vsetín'],
  ],
  '13': [['4000', 'Specializovaný finanční úřad']],
}

/** Územní pracoviště daného finančního úřadu (prázdné pole pro neznámý kód). */
export function taxOfficeBranches(office: string): TaxOfficeBranch[] {
  return (BRANCHES[office] ?? []).map(([code, name]) => ({ code, office, name }))
}

/** Finanční úřad, pod který pracoviště patří (pro předvyplnění c_ufo). */
export function officeOfBranch(branch: string): string | undefined {
  for (const [office, list] of Object.entries(BRANCHES)) {
    if (list.some(([code]) => code === branch)) return office
  }
  return undefined
}

export function taxOfficeName(code: string): string | undefined {
  return TAX_OFFICES.find((o) => o.code === code)?.name
}
