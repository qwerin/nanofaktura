# Novinky v NanoFaktuře

<!--
Tento soubor se zobrazuje přímo v aplikaci (sekce „Novinky“) — píše se pro uživatele, ne pro vývojáře.
Pravidla (viz CLAUDE.md → „Novinky“):
- Nejnovější záznam nahoře. Hlavička: `## RRRR-MM-DD · Krátký název` (datum vydání).
- Pod ní volitelně `### Podnadpis` a odrážky `- **Co** — k čemu je to dobré.` Česky, bez technického žargonu.
- Každá změna, kterou uživatel uvidí nebo pocítí, sem patří ve stejném commitu. Interní úpravy ne.
- Podporováno: odstavce, `###`, odrážky, **tučně**, *zvýraznění* (např. cesta v menu), `kód`, [odkazy](https://…). Nic dalšího.
-->

## 2026-10-04 · AI asistent

- **Připojení AI asistenta (MCP)** — v *Nastavení → API tokeny* najdete adresu a hotový příkaz pro připojení AI asistenta, třeba Claude. Asistent pak za vás najde fakturu nebo kontakt, řekne, kdo vám dluží, připraví přehled DPH, vystaví fakturu nebo zapíše náklad a úhradu. Nic neodesílá e-mailem, nic nemaže a nemění nastavení. Přihlašuje se API tokenem, takže smí jen to, co vy, a přístup kdykoli zrušíte zrušením tokenu.

## 2026-10-01 · Aplikace na plochu, hromadná úhrada, zálohy a přesnější DPH

- **Nainstalovat aplikaci** — v menu *Více* (na počítači v menu pod vaším jménem) přidáte NanoFakturu na plochu telefonu. Pak se otevírá jako běžná aplikace přes celou obrazovku, bez adresního řádku prohlížeče. Na iPhonu ukáže krátký návod (Sdílet → Přidat na plochu).
- **Označit jako uhrazené hromadně** — v seznamu faktur i nákladů je v menu „…“ akce *Označit jako uhrazené*. Uhradí najednou všechny neuhrazené doklady podle aktuálního filtru, třeba všechny staré faktury do konce loňského roku. Předem uvidíte, kolik dokladů a za kolik se označí; datum úhrady je ke dni splatnosti, nebo jedno zvolené datum. Odběratelům se přitom neposílá poděkování za úhradu.

### Opravy plateb a DPH

- **Žádné zdvojené platby** — dvojklik na „Přidat platbu“, souběh automatického párování z banky s ruční platbou nebo práce dvou lidí na jedné faktuře už nemůže zapsat platbu dvakrát ani rozhodit uhrazenou částku. Úprava položek faktury během zápisu platby se také neztratí.
- **Daňové doklady k přijatým zálohám** — plátcům DPH se ke každé platbě zálohové faktury (i spárované z banky) sám vystaví daňový doklad k přijaté platbě s vlastní řadou čísel (`ZD2026-0001`), datem platby a DPH z přijaté částky. DPH ze zálohy se tak přizná ve správném měsíci.
- **Vyúčtování zálohy** — u zálohové faktury je nová akce *Vystavit vyúčtování*, kde zadáte datum dodání. Vyúčtovací faktura převezme všechny přijaté zálohy, odečte daň už zaplacenou na daňových dokladech („Odpočet záloh“) a ukáže jen zbývající částku. Částečně zaplacená záloha se tak už nepočítá dvakrát jako dluh odběratele a doplatek se zapisuje na vyúčtovací fakturu.
- **Přesnější DPH v cizí měně** — faktura plátce v eurech (a jiných měnách) nově uvádí DPH i v korunách a použitý kurz ČNB. Faktury ze šablon, pravidelné faktury, kopie dokladů i vyúčtování si samy doplní kurz ČNB ke dni plnění, místo aby převzaly kurz 1 nebo starý kurz. Nulový kurz už nejde zadat.
- **Důvod opravy na opravném dokladu** — při vystavení dobropisu plátce zadáte důvod (třeba „Vrácení zboží“); vytiskne se na doklad i do ISDOC.
- **Datum zdanitelného plnění je u plátce povinné** — doklad bez něj už nejde uložit a starší doklady bez data se v přehledu DPH objeví s upozorněním, aby z přiznání tiše nevypadly.
- **Storno odeslané faktury plátce** — odeslaný daňový doklad už nejde stornovat, aplikace nabídne opravný doklad (dobropis). Neodeslané doklady a doklady neplátců stornovat můžete dál.
- **Plnění do EU** — u faktury s přenesenou daňovou povinností vyberete, zda jde o službu, nebo o zboží. Zboží se vykáže na správném řádku přiznání a doklad nese text o osvobození podle § 64. Slovenským odběratelům se na doklad tiskne IČ DPH. Přehled DPH připomene podání souhrnného hlášení. Identifikovaná osoba vystavuje za služby do EU fakturu – daňový doklad s datem plnění a textem „Daň odvede zákazník“.
- **Služby ze zahraničí a přenesená daň u nákladů** — u nákladu zaškrtnete *Daň přiznává odběratel* (reklama z Irska, stavební práce v tuzemsku…). Daň i odpočet se pak objeví v přiznání a kontrolním hlášení na správných řádcích. Náklady bez DPH od zahraničních dodavatelů přehled DPH označí k prověření.
- **Odpočet DPH zvlášť od daňové uznatelnosti** — u nákladu je nově samostatná volba *Uplatnit odpočet DPH*, nezávislá na tom, zda je náklad daňově uznatelný pro daň z příjmů. Stávající náklady si zachovají dosavadní nastavení.
- **Kontrolní hlášení s.r.o. měsíčně** — právnické osobě se čtvrtletním přiznáním aplikace kontrolní hlášení za čtvrtletí nevygeneruje a vyzve k měsíčnímu.
- **Drobnosti** — neuhrazené částky v seznamu faktur už nesnižují přeplatky jiných faktur, dobropisy se na nástěnce nepočítají mezi neuhrazené pohledávky, platbu dobropisu je třeba zadat se znaménkem minus, měnu doklad s platbami nejde změnit, bankovní výpisy v jenech se načtou ve správné výši, QR platba se u částek nad 9 999 999,99 Kč nevytváří, e-mail neplátce nazývá dobropis „opravná faktura“, u nákladu upozorníme, když DPH účtuje dodavatel, který není plátcem, a obnova zálohy účtu opraví nesouhlasné součty dokladů.

## 2026-09-30 · Záloha účtu, dvoufázové ověření, vyšší zabezpečení a Novinky

- **Zapomenuté heslo** — na přihlašovací stránce klikněte na „Zapomenuté heslo?“ a do e-mailu vám přijde odkaz, přes který si nastavíte nové heslo. Odkaz platí hodinu a po změně hesla se odhlásí všechna ostatní zařízení.
- **Dvoufázové ověření** — v *Nastavení → Zabezpečení* si k heslu zapnete druhý krok: kód z ověřovací aplikace v telefonu, nebo bezpečnostní klíč jako YubiKey či passkey. I když někdo zjistí vaše heslo, bez telefonu nebo klíče se nepřihlásí.
- **Záložní kódy** — při zapnutí dvoufázového ověření dostanete 10 jednorázových kódů. Uschovejte si je; pomohou, když ztratíte telefon nebo klíč.

- **Záloha celého účtu** — v *Nastavení → Záloha a přenos* stáhnete jedním tlačítkem ZIP se vším: kontakty, faktury, náklady, ceník, sklad, bankovní pohyby, šablony, historii i přílohy. Hesla a přístupové tokeny se do zálohy nikdy neukládají.
- **Obnova ze zálohy** — při zakládání nového účtu zvolte „Obnovit ze zálohy“ a účet se přenese i na jinou instanci NanoFaktury. Číslování dokladů plynule navazuje; pravidelné faktury, webhooky a automatické upomínky zůstanou po obnově vypnuté, aby nic neodešlo dvakrát.
- **Oprava: role Účetní** — účetní už nevidí tlačítka „Nová faktura“ v menu, v seznamu faktur ani „+“ ve spodní liště na mobilu; vede to jen na stránku, kde fakturu stejně vystavit nesmí.
- **Oprava: položky faktury na mobilu** — nově přidaná položka, kterou právě vyplňujete, se už nesbalí, když změníte pořadí položek.
- **Přehlednější nastavení** — sekce nastavení jsou na počítači v levém menu rozdělené do skupin *Firma a doklady*, *Tým, integrace a data* a *Můj účet*, takže je vidět všechny najednou.
- **Sekce Novinky** — právě ji čtete. Tečka u položky *Novinky* v menu prozradí, že přibylo něco nového.

### Správa instance a test e-mailu

- **Správa instance** — kdo NanoFakturu provozuje, najde v nabídce uživatele (a v *Více* na mobilu) sekci *Správa instance*: stav serveru, přehled nastavení bez hesel, upozornění na riziková nastavení a seznam uživatelů. Uživateli tu může vypnout dvoufázové ověření nebo poslat odkaz pro ověření e-mailu.
- **Test e-mailu** — v *Správa instance → Test e-mailu* jedním tlačítkem ověříte, že e-maily s fakturami opravdu odcházejí: spojení se serverem krok po kroku, šifrování, přihlášení i DNS záznamy SPF, DKIM, DMARC a MX domény odesílatele. U každého problému je česky napsané, co opravit, a nakonec přijde skutečný testovací e-mail s návodem, jak v Gmailu zkontrolovat, že neskončí ve spamu.
- **Ověření e-mailu** — správce instance musí nejdřív potvrdit svou e-mailovou adresu odkazem z e-mailu (v *Nastavení → Můj profil* nebo *Zabezpečení*). Nikdo tak nezíská správu instance jen tím, že si zaregistruje cizí adresu. Obnova zapomenutého hesla adresu ověří také.
- **Podepsané e-maily** — provozovatel může zapnout podpis e-mailů (DKIM), aby faktury u příjemců méně často končily ve spamu.
- **E-maily slovensky a německy** — k dokladům ve slovenštině a němčině se teď posílají e-maily v jejich jazyce (faktura, upomínka i poděkování za platbu), dřív česky nebo anglicky. V *Nastavení → E-maily a upomínky* upravíte nahoře texty v jazyce účtu, ostatní jazyky najdete v části *Texty pro doklady v jiných jazycích*. Při odesílání faktury je jazyk e-mailu předvybraný podle dokladu a můžete ho změnit.

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
