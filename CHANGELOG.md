# Novinky v NanoFaktuře

<!--
Tento soubor se zobrazuje přímo v aplikaci (sekce „Novinky“) — píše se pro uživatele, ne pro vývojáře.
Pravidla (viz CLAUDE.md → „Novinky“):
- Nejnovější záznam nahoře. Hlavička: `## RRRR-MM-DD · Krátký název` (datum vydání).
- Pod ní volitelně `### Podnadpis` a odrážky `- **Co** — k čemu je to dobré.` Česky, bez technického žargonu.
- Každá změna, kterou uživatel uvidí nebo pocítí, sem patří ve stejném commitu. Interní úpravy ne.
- Podporováno: odstavce, `###`, odrážky, **tučně**, *zvýraznění* (např. cesta v menu), `kód`, [odkazy](https://…). Nic dalšího.
-->

## 2026-09-30 · Záloha účtu, dvoufázové ověření, vyšší zabezpečení a Novinky

- **Zapomenuté heslo** — na přihlašovací stránce klikněte na „Zapomenuté heslo?“ a do e-mailu vám přijde odkaz, přes který si nastavíte nové heslo. Odkaz platí hodinu a po změně hesla se odhlásí všechna ostatní zařízení.
- **Dvoufázové ověření** — v *Nastavení → Zabezpečení* si k heslu zapnete druhý krok: kód z ověřovací aplikace v telefonu, nebo bezpečnostní klíč jako YubiKey či passkey. I když někdo zjistí vaše heslo, bez telefonu nebo klíče se nepřihlásí.
- **Záložní kódy** — při zapnutí dvoufázového ověření dostanete 10 jednorázových kódů. Uschovejte si je; pomohou, když ztratíte telefon nebo klíč.

- **Záloha celého účtu** — v *Nastavení → Záloha a přenos* stáhnete jedním tlačítkem ZIP se vším: kontakty, faktury, náklady, ceník, sklad, bankovní pohyby, šablony, historii i přílohy. Hesla a přístupové tokeny se do zálohy nikdy neukládají.
- **Obnova ze zálohy** — při zakládání nového účtu zvolte „Obnovit ze zálohy“ a účet se přenese i na jinou instanci NanoFaktury. Číslování dokladů plynule navazuje; pravidelné faktury, webhooky a automatické upomínky zůstanou po obnově vypnuté, aby nic neodešlo dvakrát.
- **Přehlednější nastavení** — sekce nastavení jsou na počítači v levém menu rozdělené do skupin *Firma a doklady*, *Tým, integrace a data* a *Můj účet*, takže je vidět všechny najednou.
- **Sekce Novinky** — právě ji čtete. Tečka u položky *Novinky* v menu prozradí, že přibylo něco nového.

### Správa instance a test e-mailu

- **Správa instance** — kdo NanoFakturu provozuje, najde v nabídce uživatele (a v *Více* na mobilu) sekci *Správa instance*: stav serveru, přehled nastavení bez hesel, upozornění na riziková nastavení a seznam uživatelů. Uživateli tu může vypnout dvoufázové ověření nebo poslat odkaz pro ověření e-mailu.
- **Test e-mailu** — v *Správa instance → Test e-mailu* jedním tlačítkem ověříte, že e-maily s fakturami opravdu odcházejí: spojení se serverem krok po kroku, šifrování, přihlášení i DNS záznamy SPF, DKIM, DMARC a MX domény odesílatele. U každého problému je česky napsané, co opravit, a nakonec přijde skutečný testovací e-mail s návodem, jak v Gmailu zkontrolovat, že neskončí ve spamu.
- **Ověření e-mailu** — správce instance musí nejdřív potvrdit svou e-mailovou adresu odkazem z e-mailu (v *Nastavení → Můj profil* nebo *Zabezpečení*). Nikdo tak nezíská správu instance jen tím, že si zaregistruje cizí adresu. Obnova zapomenutého hesla adresu ověří také.
- **Podepsané e-maily** — provozovatel může zapnout podpis e-mailů (DKIM), aby faktury u příjemců méně často končily ve spamu.

### Vyšší zabezpečení

- **Ochrana přihlášení** — po několika špatných pokusech o přihlášení nebo změnu hesla aplikace chvíli počká, než dovolí další. Hádání hesel tím prakticky nejde.
- **Bezpečnější pozvánky** — odkaz na pozvánku s rolí vlastníka vidí jen vlastník. Pozvánka přestane platit, pokud ten, kdo ji poslal, už nemá právo danou roli udělit.
- **Logo a razítko** — nahrát nebo smazat je může jen vlastník nebo správce. Obrázek smí mít nejvýše 2 MB a 4000 × 4000 px; větší ho aplikace odmítne a do faktury ho nevloží.
- **API tokeny s platností** — v *Nastavení → API tokeny* nově zvolíte, jak dlouho token platí. Změna hesla odhlásí ostatní zařízení a zneplatní i všechny vaše API tokeny.
- **Dlouhá hesla s diakritikou** — příliš dlouhé heslo aplikace srozumitelně odmítne místo chyby serveru.
- **Exporty do tabulek** — texty, které by tabulkový procesor mohl spustit jako vzorec, se v CSV exportu uloží jako obyčejný text.
- **Rozesílání e-mailů** — jeden e-mail s fakturou jde nejvýše 10 příjemcům a počet odeslaných e-mailů za hodinu je omezený.
- **Pro správce instance** — log serveru už nevypisuje běžné SQL dotazy ani jejich hodnoty (jen skutečné chyby databáze; podrobnější výpis zapne `NANOFAKTURA_DB_LOG`). nová instance může při první registraci vyžadovat instalační token, přihlášení funguje na HTTPS automaticky bezpečněji a aplikace posílá prohlížeči přísnější bezpečnostní pravidla. Podrobnosti jsou v návodu k nasazení.

## 2026-09-29 · Hledání, úkoly a historie

- **Rychlé hledání** — `Ctrl K` (na Macu `⌘K`) nebo lupa v hlavičce najde faktury, náklady, kontakty i položky ceníku. Diakritiku psát nemusíte. Odtud jde i rovnou založit „Novou fakturu pro…“.
- **Úkoly** — aplikace sama upozorní na fakturu po splatnosti, nespárovanou platbu nebo docházející zboží a úkol po vyřešení sama odškrtne. Vlastní úkoly si přidáte také.
- **Historie** — u každé faktury, nákladu a kontaktu vidíte, kdo co a kdy udělal. Kompletní přehled je v sekci *Aktivita*.
- **Webhooky** — pro napojení vlastních systémů: NanoFaktura pošle oznámení o vystavení, zaplacení a dalších událostech.
- **Neuhrazené faktury** — nová záložka v seznamu faktur a součty za celý vyfiltrovaný výběr, ne jen za načtenou stránku.
- **Vzhled dokladů se ukládá** — zvolená šablona PDF, barva, QR kód a vlastní patička se použijí u všech faktur. Faktury nově i ve slovenštině a němčině.
- **Srozumitelnější chybové hlášky** — v češtině a s vysvětlením, co s tím dělat.
- **Identifikovaná osoba k DPH** — tuzemské faktury se správně vystavují bez DPH.

## 2026-09-28 · Banka, přehledy a DPH

- **Napojení na Fio banku** — platby se stahují samy každé dvě hodiny a spárují s fakturou podle variabilního symbolu a částky. Kde si aplikace není jistá, nabídne návrh ke schválení jedním klepnutím.
- **Import výpisů** — ABO/GPC a CSV z Fio, ČSOB, Komerční banky a Air Bank.
- **Kurzy ČNB** — u dokladů v cizí měně se kurz doplní sám.
- **Kontrola dodavatelů** — upozornění na nespolehlivého plátce DPH a na účet, který není zveřejněný v registru.
- **Přehledy** — tržby, náklady a zisk po měsících, nejlepší odběratelé, průměrná doba úhrady a srovnání skutečných výdajů s paušálem pro daňové přiznání OSVČ.
- **DPH pro plátce** — podklady pro přiznání k DPH a kontrolní hlášení včetně XML pro podání přes EPO.
- **Exporty pro účetní** — seznamy do Excelu a CSV, ZIP se všemi PDF za období, faktury ve formátu ISDOC.
- **Odkaz pro klienta** — každá faktura má vlastní stránku s QR platbou a stažením PDF. Uvidíte, kdy si ji klient otevřel.

## 2026-09-27 · Náklady, pravidelné faktury a tým

- **Náklady** — evidence přijatých dokladů včetně plateb. Účtenku stačí vyfotit telefonem.
- **Ceník a sklad** — položky do faktur jedním klepnutím, sklad se při fakturaci odepíše sám a upozorní na nízký stav.
- **Pravidelné faktury** — nastavte jednou a NanoFaktura je vystaví sama; do textu lze vložit třeba `{MONTH_NAME}` a doplní se aktuální měsíc.
- **Odesílání e-mailem** — faktura s PDF přímo klientovi, automatické upomínky po splatnosti a poděkování za platbu.
- **Tým a role** — pozvěte kolegu nebo účetní; role Vlastník, Administrátor, Člen a Účetní (jen čtení).
- **Logo a razítko** — nahrajte je v *Nastavení → Vzhled dokladů* a objeví se na každé faktuře.

## 2026-09-26 · Nová NanoFaktura

Aplikace je postavená znovu od základu — rychlejší, přehlednější a od začátku navržená pro mobil.

- **Faktury, zálohové faktury a dobropisy** — s automatickým výpočtem DPH, číselnými řadami a vlastním formátem čísla.
- **Kontakty s ARES** — stačí zadat IČO a údaje o firmě se doplní samy.
- **QR platba a profesionální PDF** — tři šablony, klient zaplatí načtením QR kódu v bankovní aplikaci.
- **Platby a přehled** — dashboard s tržbami, neuhrazenými fakturami a fakturami po splatnosti.
- **Pohodlně na mobilu** — spodní menu, velká tlačítka a možnost přidat si aplikaci na plochu telefonu. K dispozici je i tmavý režim.
