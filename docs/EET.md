# EET 2.0 — poznatky

Podklady, ze kterých vychází implementace evidence tržeb (`internal/eet`, `internal/api/eet.go`, SPEC §7.19).
Stav k 13. 11. 2026. Co je závazné pro kód, je v `docs/SPEC.md`; tady je kontext, zdroje a co zůstává otevřené.

> Právní část je sepsaná z technické dokumentace Finanční správy a z odborných článků, **ne z textu zákona**.
> Hraniční případy (dobírka, karta přes platební bránu, zastoupení) je potřeba ověřit v zákoně nebo u daňového poradce.

## 1. Právní rámec

- **Zákon č. 180/2026 Sb.**, o evidenci tržeb. Účinnost **1. 1. 2027** pro všechny najednou (bez fázování); leden 2027 je
  označovaný jako pilotní měsíc. Ustanovení o přípravě (certifikáty, evidenční jednotky) platí dřív — od **1. 11. 2026**
  jde v portálu MOJE daně aktivovat EET a stáhnout certifikát.
- **Kdo eviduje:** poplatníci daně z příjmů fyzických i právnických osob (bez vazby na plátcovství DPH), kteří mají
  evidované tržby na území ČR.
- **Co je evidovaná tržba:** platba, která zakládá příjem z podnikání **a** je *kontaktní platbou*:
  1. při osobním kontaktu s poplatníkem (nebo jeho zaměstnancem),
  2. v provozovně nebo dopravním prostředku poplatníka (i samoobslužně),
  3. mimo ně, pokud má platba „hmotnou podstatu“ — typicky hotovost.
- **Forma platby nerozhoduje** — hotovost, karta, QR, i bankovní převod, pokud proběhne kontaktně. Běžný převod na dálku
  (úhrada faktury z domu) evidovanou tržbou není. Rozhoduje **skutečný způsob úhrady**, ne forma uvedená na faktuře.
  Příklad Finanční správy: faktura na 1 000 Kč, 200 Kč kartou v provozovně + 800 Kč druhý den převodem → eviduje se jen 200 Kč.
- **Výjimky:** stát, obce, příspěvkové organizace, banky a pojišťovny, nevidomí a hluchoslepí, prodejní automaty, veřejné
  toalety, jízdné, poštovní služby, energetika a další. **„EET OFF“**: OSVČ v 1. pásmu paušálního režimu s příjmy do
  1 mil. Kč se může z evidence vyvázat příplatkem 1 400 Kč měsíčně k paušální dani.
- **Účtenka:** zákon o evidenci tržeb ji nevyžaduje (povinnost vydat doklad může plynout z jiných předpisů).
- **Lhůta:** údaje se posílají nejpozději při uskutečnění tržby. Když odpověď nepřijde v mezní době odezvy (nejméně 2 s),
  posílá se znovu bezodkladně po odstranění příčiny, **nejpozději do 48 hodin**. Povinnost zaslat trvá i po lhůtě.

## 2. Co potřebuje poplatník

Vše v portálu **MOJE daně → DIS+ → Evidence tržeb**:

| Údaj | Odkud | V aplikaci |
|---|---|---|
| **EIČ** (evidenční identifikační číslo) | = DIČ; fyzická osoba bez DIČ `CZ` + rodné číslo, bez RČ identifikátor z DIS+ | `eic`, formát `CZ` + 8–10 číslic |
| **Číslo evidenční jednotky** | přidělí DIS+ po založení jednotky (provozovna, web/aplikace, vozidlo, „zbytek“) | `unit_id`, 1–999999999 |
| **Označení pokladny** | volí si poplatník | `register_id`, max 20 znaků, výchozí `1` |
| **Pokladní certifikát** | certifikační autorita EET, soubor `.p12` + heslo, platnost 366 dní | nahrává se, ukládá šifrovaně |

Jeden certifikát lze použít pro více pokladen. Změnu údajů o evidenční jednotce je nutné oznámit do 15 dnů.
Pro drobné podnikatele (max 2 jednotky, 2 zaměstnanci) nabízí stát webovou aplikaci MOJE EET.

## 3. Datové rozhraní (verze 4.1)

- **Protokol:** SOAP 1.1 přes HTTPS, jediná synchronní operace `OdeslaniTrzby`.
  `SOAPAction: http://fs.gov.cz/eet/OdeslaniTrzby`, `Content-Type: text/xml; charset=utf-8`.
  TLS ≥ 1.2, bez klientského certifikátu na úrovni TLS, HTTP/1.1.
- **Endpointy:**
  - produkce `https://trzbyeet.gov.cz/eet/services/EETServiceSOAP/v4`
  - playground `https://pg.trzbyeet.gov.cz/eet/services/EETServiceSOAP/v4`
  - Produkce má DNS balancing: používat jméno, ctít TTL, překládat DNS při každém novém spojení (klient nedrží keep-alive).
- **Jmenný prostor:** `http://fs.gov.cz/eet/schema/v4` (hlavní verze je součástí URL i jmenného prostoru).
- **Limit:** celá zpráva včetně obálky max 12 kB. Jen ASCII znaky (9, 10, 13, 32–126).

### Zpráva o tržbě

```xml
<tns:Trzba>
  <tns:Hlavicka uuid_zpravy="…" dat_odesl="2027-01-08T21:19:40+01:00" prvni_zaslani="true" overeni="true"/>
  <tns:Data eic_popl="CZ00000019" id_jednotky="181" id_pokl="1" porad_cis="42"
            dat_trzby="2027-01-07T22:01:00+01:00" celk_trzba="1234.50"/>
</tns:Trzba>
```

| Atribut | Povinný | Formát |
|---|---|---|
| `uuid_zpravy` | ano | UUID (doporučena v4); **nové pro každé odeslání**, i opakované |
| `dat_odesl` | ano | `rrrr-mm-ddThh:mm:ss±hh:mm`, český čas s offsetem (+01:00 / +02:00) |
| `prvni_zaslani` | ano | `true` první pokus, `false` opakované zaslání téže tržby |
| `overeni` | ne | `true` = ověřovací mód (nic se neeviduje) |
| `eic_popl` | ano | `CZ[0-9]{8,10}` |
| `id_jednotky` | ano | `[1-9][0-9]{0,8}` |
| `id_pokl` | ano | `[0-9a-zA-Z.,:;/#\-_ ]{1,20}` |
| `porad_cis` | ano | `[0-9a-zA-Z.,:;/#\-_ ]{1,25}` |
| `dat_trzby` | ano | jako `dat_odesl` |
| `celk_trzba` | ano | Kč, právě 2 desetinná místa, tečka, bez vedoucích nul, ne `-0.00`; \|x\| < 100 mil. |
| `eic_poverujiciho`, `povereni_vice_popl` | ne | evidence v pověření — **nepodporujeme** |
| `urceno_cerp_zuct`, `cerp_zuct` | ne | nabití / čerpání (kupóny, čipy) — **nepodporujeme** |

**Totožnost tržby** určuje šestice `eic_popl | id_jednotky | id_pokl | porad_cis | dat_trzby | celk_trzba`. Zpráva se stejnými
hodnotami je totéž co už přijatá tržba, takže opakované odeslání nevytvoří duplicitu.

Oproti původní EET zmizely kódy BKP/PKP, režim běžný/zjednodušený a všechny položky DPH.

### Odpověď

- **Potvrzení:** `<Potvrzeni pok="…"/>` — POK = UUID + `-` + dvě hex číslice zařízení (39 znaků). Mimo produkci má
  `test="true"` a POK končí `-ff` (není platný).
- **Chyba:** `<Chyba kod="…">text</Chyba>` (bez podpisu). Až 10 elementů `<Varovani kod_varov="…">`.
- Hlavička odpovědi `X-Global-Transaction-Id` — hodí se pro dotazy na podporu, ukládáme ji.

| Kód chyby | Význam | Co dělá aplikace |
|---|---|---|
| `-1` (a další záporné) | dočasná technická chyba | zkusí později |
| `0` | ověřovací mód prošel | test spojení OK |
| `2` | neplatné kódování XML | odmítnuto → `failed` |
| `3` | zpráva nevyhověla XML schématu | `failed` |
| `4` | neplatný podpis (i propadlý / zneplatněný certifikát) | `failed` |
| `6` | EIČ má chybnou strukturu | `failed` |
| `7` | zpráva je příliš velká | `failed` |
| `8` | technická chyba nebo chyba dat | `failed` |

| Kód varování | Význam (tržba je přesto přijata) |
|---|---|
| 1 | EIČ ve zprávě se neshoduje s EIČ v certifikátu |
| 2 | chybný formát EIČ pověřujícího |
| 4 | datum tržby je novější než přijetí zprávy (o víc než 2 h) |
| 5 | datum tržby je výrazně v minulosti (víc než 2 roky, nebo před minimem prostředí) |
| 6 | `id_jednotky` neodpovídá tvaru z DIS+ (aspoň 2 cifry, poslední 1–4) |
| 7 | EIČ pověřujícího je stejné jako EIČ poplatníka |

Minimální datum tržby: playground 1. 7. 2026, produkce 1. 1. 2027 (od 1. 11. 2026 přechodný režim — starší datum vrátí jen varování 5).

### Podpis

WS-Security 1.1 + XML-DSig; podepisuje se **jen `soap:Body`**:

- kanonizace Exclusive C14N (`http://www.w3.org/2001/10/xml-exc-c14n#`),
- digest SHA-256, podpis RSA-SHA256,
- certifikát v `wsse:BinarySecurityToken` (X509v3, Base64), odkaz přes `SecurityTokenReference`,
- žádné další hlavičky (Timestamp, WS-Addressing) — větší zprávy mohou být odmítnuty jako útok.

Klient nepoužívá XML knihovnu: `Body` i `SignedInfo` generuje rovnou v kanonickém tvaru (jmenné prostory na kořeni
podstromu seřazené podle prefixu, atributy abecedně, prázdné elementy jako `<a></a>`), takže odeslané bajty jsou přesně ty,
ze kterých se počítá otisk. Správnost těchto pravidel hlídá test `TestOfficialSampleCanonicalForm` nad oficiální vzorovou
zprávou (`internal/eet/testdata/sample.v4.req.xml`).

### Certifikáty

- Pokladní certifikát: RSA 2048, platnost 366 dní, `CN` = EIČ, KeyUsage digitalSignature + nonRepudiation.
  Hierarchie: *EETv2 NCA Root CA RSA* (RSA 4096, 10 let) → *EETv2 NCA SubCA RSA* (RSA 2048, 4 roky).
- CRL: `https://caeet.gov.cz/crldp` (vydání každých 8 h, platnost 24 h). Playground CRL nemá.
- Serverový TLS certifikát: DigiCert, typ EV. Potvrzovací zprávy podepisuje certifikát vydaný *I.CA Public CA/RSA 06/2022*.
- CA EET má API pro automatickou obnovu certifikátu (JWT) — **nepoužíváme**, certifikát se nahrává ručně.

## 4. Postupy pro zvláštní situace (podle Finanční správy)

- **Opakované zaslání:** data tržby beze změny; mění se jen `uuid_zpravy`, `dat_odesl` a `prvni_zaslani` (→ `false`).
- **Storno:** nová zpráva se zápornou částkou a **aktuálním** časem; na původní tržbu se nijak neodkazuje.
- **Oprava:** storno + nová správná tržba, nebo jedna „rozdílová“ zpráva.
- **Zaokrouhlení, sleva, spropitné:** eviduje se skutečně přijatá částka (78,90 Kč v hotovosti zaokrouhlené na 79 Kč → 79,00).
- **Záloha a doplatek:** každá platba se eviduje samostatně, nejsou provázané.
- **Cizí měna:** částka se uvádí v Kč, přepočet podle obecných pravidel pro určení příjmu.
- **Kauce:** složení ani vrácení se neeviduje; eviduje se až její započtení, pokud byla složena kontaktně.
- **Poukaz na konkrétní zboží/službu:** eviduje se jen prodej poukazu, jeho uplatnění už ne.

## 5. Jak to řeší NanoFaktura

- Evidence se týká **plateb faktur** označených jako kontaktní (`PaymentCreate.eet`). Bez výslovné hodnoty se eviduje jen
  platba faktury s formou úhrady *hotově*; ve formuláři je přepínač předzaškrtnutý i pro kartu.
- `porad_cis` = ID záznamu tržby. `dat_trzby` = okamžik zápisu platby; u platby zapsané zpětně poledne daného dne
  (skutečný čas neznáme). Budoucí datum úhrady se s evidencí odmítne.
- Tržba vzniká v transakci platby, odesílá se hned po commitu (timeout 5 s) a pak úlohou `eet` každou minutu
  s prodlevami 1, 2, 5, 10, 30, 60 min, dál každou hodinu bez omezení.
- Kritická chyba → stav `failed`, událost `eet.failed`, úkol u dokladu. Nahrání nového certifikátu vrátí odmítnuté tržby do fronty.
- Smazání platby: zaevidovaná nebo možná doručená tržba → storno; nikdy neodeslaná nebo odmítnutá → jen zrušena.
- Hromadná úhrada, párování z banky a platby převzaté ze zálohy tržbu nezakládají.
- Nastavení a certifikát nejsou v záloze účtu; historie tržeb ano.

## 6. Otevřené body

- **Podpis odpovědi neověřujeme** (jen TLS, tvar POK a shodu příznaku `test` s prostředím).
- **Dobírka a karta přes platební bránu** — zdroje je uvádějí jako případy vyžadující pozornost, bez jasného závěru.
  Necháváme na uživateli.
- **Evidence v pověření / zastoupení** a **nabití/čerpání** nejsou podporované.
- **Jedna evidenční jednotka a jedna pokladna na účet.**
- **Automatická obnova certifikátu** přes API CA EET chybí; aplikace jen upozorní 30 dní před koncem platnosti.
- **POK není na PDF faktury** ani se netiskne účtenka.
- Popis rozhraní v1.0–1.2 vznikl podle vládního návrhu zákona (sněmovní tisk 189/0). Po vyhlášení zákona je vhodné
  zkontrolovat na eet.gov.cz, jestli nevyšla novější verze dokumentace nebo rozhraní.

## 7. Testování

- **Unit a API testy** používají falešný přijímací systém `internal/eet/eettest` (ověřuje podpis, umí výpadek, dočasnou
  chybu a odmítnutí) a certifikáty generované za běhu.
- **Playground Finanční správy** — sdílené testovací certifikáty jsou ke stažení na eet.gov.cz
  (`CAEET_Playground_2026_v1.zip`: EIČ `CZ00000019` právnická osoba, `CZ8551015704` fyzická osoba, `CZ683555118` poplatník
  s vlastním identifikátorem; heslo je v ZIPu):

  ```bash
  EET_PLAYGROUND_P12=/cesta/CA_EET-Playground-CZ00000019.p12 EET_PLAYGROUND_PASSWORD=… \
    go test ./internal/eet/ -run TestPlayground -v -count=1
  ```

  Test odešle tržbu, ověřovací zprávu a storno. Ověřeno 13. 11. 2026 — playground vše potvrdil.
- V aplikaci lze účet přepnout na prostředí *playground* a zkoušet nanečisto; POK z něj končí `-ff`.

## 8. Zdroje

Oficiální (eet.gov.cz → [Pro vývojáře](https://eet.gov.cz/cs/pro-vyvojare) → Dokumenty ke stažení):

- [Formát a struktura údajů o evidované tržbě a popis datového rozhraní, v1.2](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/EET_popis_rozhrani_v1.2.pdf)
- [WSDL](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/EETServiceSOAP.wsdl), [XSD](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/EETXMLSchema.xsd), [vzorová zpráva](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/CZ00000019.eet.v4.req.xml)
- [Popis položek datové zprávy a příklady situací](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/EET_popis_polozek_datove_zpravy.pdf)
- Přístupové a provozní informace: [produkce](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/EET_pristupove_provozni_informace_produk-3.pdf), [playground](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/EET_pristupove_provozni_informace_playgr-3.pdf)
- [Prezentace ze semináře pro vývojáře (23. 6. 2026)](https://eet.gov.cz/assets/cs/cmsmedia/pro-vyvojare/EET2_Prezentace_Seminar_pro_vyvojare_23-.pdf)
- Podpora: `epodpora@fs.gov.cz`

Sekundární (právní shrnutí): [TPA](https://www.tpa-group.cz/news/zakon-o-eet-2-0-vcetne-pridruzenych-novel-zdph-a-zdp-byl-schvalen/),
[BDO](https://www.bdo.cz/cs-cz/sluzby/dane/danove-poradenstvi-pro-pravnicke-osoby/eet-2-0-prehledne),
[Podnikatel.cz](https://www.podnikatel.cz/clanky/financni-sprava-zverejnila-technicke-informace-a-dokumentaci-k-eet-2-0/).
