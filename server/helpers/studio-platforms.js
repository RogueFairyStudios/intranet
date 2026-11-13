const storefronts = [
  'steam',
  'epic',
  'apple_app_store',
  'google_play',
  'playstation',
  'xbox',
  'nintendo',
  'igdb',
  'itch_gog',
  'bgg_tabletop',
  'custom'
]

const submissionStatuses = [
  'preparing',
  'in_review',
  'changes_requested',
  'approved',
  'live'
]

const itemStatuses = ['pending', 'in_progress', 'done', 'na']
const checkCategories = ['technical', 'bureaucratic']

function item (ptTitle, enTitle, ptSpec, enSpec, required = true) {
  return { ptTitle, enTitle, ptSpec, enSpec, required }
}

function asset (name, dimensions, format, ptSpec, enSpec, required = true) {
  return { name, dimensions, format, ptSpec, enSpec, required }
}

const catalog = {
  steam: {
    label: { pt: 'Steam (Steamworks)', en: 'Steam (Steamworks)' },
    technical: [
      item(
        'Build SteamPipe, Depots e Launch Options configurados',
        'SteamPipe build, Depots and Launch Options configured',
        'Validar inicialização pelo cliente Steam nos SOs suportados, Steamworks SDK ativo e redistribuíveis declarados.',
        'Verify launch through Steam client across supported OSes, active Steamworks SDK, and declared redistributables.'
      ),
      item(
        'Steam Cloud / Auto-Cloud isolado por conta',
        'Steam Cloud / Auto-Cloud isolated per account',
        'Confirmar caminhos de save separados por AccountID e sincronização entre PC e portátil.',
        'Confirm save paths isolated by AccountID and cross-device sync between desktop and handheld.'
      ),
      item(
        'Steam Input, Conquistas e compatibilidade Steam Deck',
        'Steam Input, Achievements and Steam Deck compatibility',
        'Testar glifos de controle, resolução 1280×800, legibilidade de fonte e ausência de bloqueadores para selo Playable/Verified.',
        'Test controller glyphs, 1280×800 resolution, text legibility, and absence of blockers for Playable/Verified rating.'
      ),
      item(
        'Ausência de links ou SDKs de lojas concorrentes na build',
        'No competing storefront links or SDKs in build',
        'Garantir que a build enviada ao SteamPipe não contenha menções ou URLs de compra externas não permitidas.',
        'Ensure the SteamPipe build contains no prohibited external store purchase links or overlays.'
      )
    ],
    bureaucratic: [
      item(
        'Questionário de conteúdo Steam e ClassInd / ESRB / PEGI',
        'Steam content survey and ClassInd / ESRB / PEGI ratings',
        'Preencher questionário de maturidade da Valve e alinhar descritores oficiais (incluindo ClassInd para o Brasil).',
        'Complete Valve content survey and align official age rating descriptors (including Brazil ClassInd).'
      ),
      item(
        'Cadastro fiscal (W-8BEN-E), KYC e dados bancários Steamworks',
        'Tax onboarding (W-8BEN-E), KYC and Steamworks payout info',
        'Verificar situação fiscal, retenção internacional e titularidade societária da Rogue Fairy Studios.',
        'Verify tax withholding forms, company KYC, and Rogue Fairy Studios banking details.'
      ),
      item(
        'EULA, Política de Privacidade (LGPD/GDPR), IA e licenças de terceiros',
        'EULA, Privacy Policy (LGPD/GDPR), AI disclosure and third-party licenses',
        'Confirmar direitos de uso de fontes, trilhas, middlewares e declaração obrigatória da Valve sobre IA generativa.',
        'Confirm font, music, and middleware licenses plus Valve mandatory generative AI disclosure if applicable.'
      ),
      item(
        'Página "Em Breve" aprovada cumprindo janela mínima de pré-lançamento',
        '"Coming Soon" store page approved within minimum pre-release window',
        'Revisão final da página da loja concluída com antecedência mínima exigida pelo Steamworks antes do lançamento.',
        'Store presence review approved ahead of Valve minimum Coming Soon duration prior to launch.'
      )
    ],
    assets: [
      asset('Header Capsule', '920×430 px', 'PNG / JPG', 'Arte principal com logo legível; proibido incluir notas de review ou selos de prêmios.', 'Key art with legible game logo; no review scores or award badges allowed.'),
      asset('Small Capsule', '462×174 px', 'PNG / JPG', 'Logo em destaque legível em listas compactas e resultados de busca.', 'High-contrast game logo legible at compact list sizes.'),
      asset('Main Capsule', '1232×706 px', 'PNG / JPG', 'Peça principal para o carrossel de destaque da página inicial da Steam.', 'Primary promotional artwork and logo for home page featured carousel.'),
      asset('Vertical Capsule', '748×896 px', 'PNG / JPG', 'Cápsula vertical utilizada em promoções sazonais e eventos temáticos.', 'Vertical key art with logo used on seasonal sales and themed events.'),
      asset('Library Capsule', '600×900 px', 'PNG / JPG', 'Capa vertical exibida na grade da biblioteca do jogador (com título/logo).', 'Vertical cover displayed in the user Steam Library grid (with game title/logo).'),
      asset('Library Hero', '3840×1240 px', 'PNG / JPG', 'Somente arte limpa SEM texto ou logo; manter foco visual na zona segura central 860×380 px.', 'Artwork only with strictly NO text or logos; keep focal elements inside central 860×380 px safe zone.'),
      asset('Library Logo', '1280×720 px (max)', 'PNG (Alpha)', 'Logo em fundo transparente posicionado sobre o Library Hero.', 'Transparent PNG game logo overlaid on top of the Library Hero.'),
      asset('App & Client Icons', '256×256 px + ICO', 'ICO / PNG', 'Ícone quadrado de atalho do executável e ícone 184×184 px do cliente Steam.', 'Executable desktop .ico/.icns and 184×184 px Steam client icon.'),
      asset('Gameplay Screenshots (5+)', '1920×1080 px (16:9)', 'PNG / JPG', 'Mínimo de 5 capturas reais de gameplay em alta definição sem textos promocionais.', 'Minimum 5 widescreen high-res screenshots showing actual in-game gameplay.'),
      asset('Store Gameplay Trailer', '1920×1080 px 60fps', 'MP4 (H.264 / AAC)', 'Trailer principal de gameplay com metadados e sinopse localizados (pt-BR / en).', 'Primary gameplay trailer encoded in MP4 H.264/AAC with localized store copy.')
    ]
  },
  epic: {
    label: { pt: 'Epic Games Store', en: 'Epic Games Store' },
    technical: [
      item(
        'Upload via BuildPatchTool (BPT) e validação nos artefatos Staging/Live',
        'BuildPatchTool (BPT) upload and Staging/Live artifact verification',
        'Testar instalação, atualização diferencial e execução pelo Epic Games Launcher.',
        'Verify installation, delta patching, and launch arguments via Epic Games Launcher.'
      ),
      item(
        'Integração Epic Online Services (EOS) e paridade cross-play',
        'Epic Online Services (EOS) integration and cross-play parity',
        'Validar EOS Overlay, conquistas, cloud saves e paridade multiplayer entre lojas PC.',
        'Validate EOS Overlay, achievements, cloud saves, and PC cross-store multiplayer parity.'
      )
    ],
    bureaucratic: [
      item(
        'Certificação global IARC vinculada no Epic Developer Portal',
        'Global IARC rating linked in Epic Developer Portal',
        'Obter classificação automática IARC cobrindo ClassInd (Brasil), ESRB, PEGI, USK e GRAC.',
        'Obtain IARC rating certificate covering ClassInd, ESRB, PEGI, USK, and GRAC.'
      ),
      item(
        'Contrato EGS, Hyperwallet fiscal/payout e Política de Privacidade HTTPS',
        'EGS distribution agreement, tax/payout profile and HTTPS Privacy Policy',
        'Confirmar conta de recebimento ativa, acordos assinados e URL pública de privacidade.',
        'Confirm active payout account, signed agreements, and public HTTPS privacy policy URL.'
      )
    ],
    assets: [
      asset('Offer Image Landscape (Wide)', '2560×1440 px (16:9)', 'JPG / PNG', 'Imagem horizontal da oferta com logo do produto centralizado e legível.', 'Landscape offer artwork with legible centered product logo.'),
      asset('Offer Image Portrait (Tall)', '1200×1600 px (3:4)', 'JPG / PNG', 'Capa vertical da oferta e da biblioteca na Epic Games Store.', 'Portrait offer and library tile image with product logo.'),
      asset('Product Logo', '1280×720 px', 'PNG (Alpha)', 'Logo isolado com transparência para vitrine e página do produto.', 'Transparent PNG product logo for store PDP header.'),
      asset('Store Carousel Screenshots (5+)', '1920×1080 px (16:9)', 'JPG / PNG', 'Mínimo de 5 capturas de gameplay limpas em 16:9.', 'Minimum 5 clean 16:9 gameplay screenshots.'),
      asset('Epic Store Trailer', '1920×1080 px', 'MP4 (H.264)', 'Vídeo sem marcas ou botões de lojas concorrentes.', 'Promotional gameplay trailer free of competing platform branding.')
    ]
  },
  apple_app_store: {
    label: { pt: 'Apple App Store (iOS / macOS)', en: 'Apple App Store (iOS / macOS)' },
    technical: [
      item(
        'Assinatura de código, Notarization (macOS) e build validada no TestFlight',
        'Code signing, macOS Notarization and validated TestFlight build',
        'Verificar certificados de distribuição, entitlements, arquiteturas arm64 e ausência de APIs privadas.',
        'Verify distribution certificates, entitlements, arm64 slices, and absence of private APIs.'
      ),
      item(
        'App Store Review Guidelines: Sign in with Apple, exclusão de conta e StoreKit',
        'App Store Review Guidelines: Sign in with Apple, account deletion and StoreKit',
        'Confirmar restauração de compras, exclusão de conta in-app (quando aplicável) e layout em Dynamic Island / Safe Areas.',
        'Confirm Restore Purchases, in-app account deletion (if accounts exist), and Dynamic Island safe areas.'
      ),
      item(
        'PrivacyInfo.xcprivacy (Required Reason APIs) e App Tracking Transparency',
        'PrivacyInfo.xcprivacy manifest and App Tracking Transparency (ATT)',
        'Incluir manifesto de privacidade para o app e todos os SDKs de terceiros embutidos.',
        'Bundle accurate PrivacyInfo.xcprivacy manifests for app code and all embedded third-party SDKs.'
      )
    ],
    bureaucratic: [
      item(
        'Classificação etária App Store Connect + ClassInd Brasil + status Trader UE (DSA)',
        'App Store Connect age rating + Brazil ClassInd + EU DSA Trader status',
        'Completar questionário regulatório global, URL de suporte e URL de Política de Privacidade.',
        'Complete global age rating questionnaire, support URL, privacy policy URL, and EU DSA trader verification.'
      ),
      item(
        'App Privacy ("Nutrition Labels"), Paid Apps Agreement e credenciais para App Review',
        'App Privacy Nutrition Labels, Paid Apps Agreement and App Review demo notes',
        'Declarar coleta de dados com precisão e fornecer conta/instruções de teste para a equipe de revisão da Apple.',
        'Declare data collection accurately and provide test credentials/notes for Apple App Review.'
      )
    ],
    assets: [
      asset('App Store Icon', '1024×1024 px', 'PNG (No Alpha)', 'PNG sRGB/P3 quadrado sem canal alfa/transparência e sem arredondamento manual.', 'Square sRGB/P3 PNG strictly without alpha transparency or pre-rounded corners.'),
      asset('iPhone 6.9" Screenshots (3–10)', '1320×2868 px / 2868×1320 px', 'PNG / JPG', 'Capturas master da classe 6.9" (retrato ou paisagem) usadas para derivação automática.', 'Master 6.9-inch display class screenshots (portrait or landscape).'),
      asset('iPad Pro 13" Screenshots (3–10)', '2064×2752 px / 2752×2064 px', 'PNG / JPG', 'Obrigatório caso o binário suporte iPadOS / Universal.', 'Required when shipping Universal / iPadOS builds.'),
      asset('App Preview Video', '1920×886 / 1080×1920 px (≤30s)', 'MOV / MP4 (H.264)', 'Vídeo de gameplay real (até 30s) com faixa de áudio estéreo.', 'Up to 30-second real app/gameplay footage with stereo audio track.', false),
      asset('App Store Copy & Keywords', 'Title ≤30 · Subtitle ≤30 · Keywords ≤100', 'Text / Markdown', 'Textos localizados, notas da versão e palavras-chave revisadas.', 'Localized title, subtitle, promotional text, description, and keyword list.')
    ]
  },
  google_play: {
    label: { pt: 'Google Play Store (Android)', en: 'Google Play Store (Android)' },
    technical: [
      item(
        'Android App Bundle (.aab), Play App Signing, targetSdk atual e páginas de 16 KB',
        'Android App Bundle (.aab), Play App Signing, current targetSdk and 16 KB page size',
        'Validar alinhamento ELF de bibliotecas nativas para páginas de 16 KB, Play Asset Delivery (se >200 MB) e Play Billing.',
        'Verify native .so 16 KB ELF alignment, Play Asset Delivery (if >200 MB), and Play Billing compliance.'
      ),
      item(
        'Pre-launch Report aprovado (Android Vitals, ANR/Crash e telas grandes)',
        'Pre-launch Report clean (Android Vitals, ANR/Crash thresholds and large screens)',
        'Corrigir falhas automáticas em dispositivos físicos do Firebase Test Lab no Play Console.',
        'Resolve automated test issues across Play Console physical test devices.'
      )
    ],
    bureaucratic: [
      item(
        'Questionário IARC (ClassInd/ESRB/PEGI), público-alvo e declaração de AD_ID',
        'IARC rating questionnaire, target audience declaration and AD_ID disclosure',
        'Emitir classificação IARC no Play Console e declarar conformidade com Políticas para Famílias se aplicável.',
        'Generate Play Console IARC certificates and submit Target Audience & Content declarations.'
      ),
      item(
        'Formulário Segurança dos Dados (Data Safety), verificação DUNS e exclusão de conta web',
        'Data Safety form, developer organization verification and web account deletion URL',
        'Preencher criptografia/retenção de dados, política de privacidade e URL externa de exclusão de dados.',
        'Complete Data Safety disclosures, verified developer profile, and web data deletion link.'
      )
    ],
    assets: [
      asset('Google Play Hi-Res Icon', '512×512 px', 'PNG 32-bit (≤1 MB)', 'Ícone em sRGB respeitando a zona segura central sem textos minúsculos.', '32-bit sRGB PNG icon ≤1024 KB respecting Google Play safe zone.'),
      asset('Feature Graphic', '1024×500 px', 'JPG / PNG 24-bit', 'Arte horizontal obrigatória da vitrine e do trailer (sem transparência alfa).', 'Mandatory 1024×500 banner displayed on store listing and video preview (no alpha).'),
      asset('Phone Screenshots (2–8)', '1920×1080 px / 1080×1920 px', 'PNG / JPG', 'Mínimo de 2 capturas (recomendado 4+) demonstrando a experiência real do jogo.', '2 to 8 16:9 or 9:16 screenshots showcasing actual in-game experience.'),
      asset('7" & 10" Tablet Screenshots', '1920×1200 px & 2560×1600 px', 'PNG / JPG', 'Capturas de alta resolução para elegibilidade na vitrine de tablets/Chromebooks.', 'High-res tablet screenshots for large-screen merchandising eligibility.', false),
      asset('Promo Video (YouTube URL)', '16:9 YouTube Public/Unlisted', 'HTTPS URL', 'URL oficial do vídeo no YouTube sem anúncios ou restrição de incorporação.', 'Public or unlisted YouTube trailer URL with embedding enabled.', false)
    ]
  },
  playstation: {
    label: { pt: 'Sony PlayStation Store (PS5 / PS4)', en: 'Sony PlayStation Store (PS5 / PS4)' },
    technical: [
      item(
        'Conformidade integral com PlayStation TRC (Technical Requirements Checklist)',
        'Full PlayStation TRC (Technical Requirements Checklist) compliance',
        'Validar gerenciamento de sessão/usuários, suspensão/retomada, Save Data, rede PSN e pacote master no Partner Portal.',
        'Validate user session handling, suspend/resume, Save Data, PSN networking, and master package checks.'
      ),
      item(
        'Terminologia oficial DualSense, Troféus e Activity Cards (PS5)',
        'Official DualSense terminology, Trophies and PS5 Activity Cards',
        'Revisar nomenclatura exata de botões/acessórios Sony, sincronização de Troféus e metadados do sistema.',
        'Audit official Sony controller terminology/icons, Trophy set sync, and PS5 system metadata.'
      )
    ],
    bureaucratic: [
      item(
        'Certificados etários regionais (ClassInd, ESRB, PEGI, CERO / IARC) e avisos de saúde',
        'Regional age rating certificates (ClassInd, ESRB, PEGI, CERO / IARC) and health warnings',
        'Submeter certificados oficiais por região SIEA/SIEE/SIEJA e telas obrigatórias de fotossensibilidade.',
        'Submit official age rating certificates per SIE region and mandatory health/photosensitivity screens.'
      ),
      item(
        'Aprovação de Concept, Title ID, precificação regional e revisão legal SIE',
        'Concept approval, Title ID setup, regional pricing and SIE legal clearance',
        'Confirmar aprovação comercial/jurídica do produto no PlayStation Partner Hub.',
        'Confirm product commercial, legal, and territory configuration in PlayStation Partner Hub.'
      )
    ],
    assets: [
      asset('PlayStation Master / Concept Icon', '1024×1024 px', 'PNG / PSD Template', 'Ícone principal seguindo estritamente as margens seguras do template do PlayStation Partner Portal.', 'Master game icon strictly following PlayStation Partner Portal safe-zone templates.'),
      asset('Store Portrait Cover Art', '1200×1600 px (3:4)', 'PNG / JPG', 'Arte vertical com logo para listagens da PlayStation Store.', 'Portrait cover art with logo for PlayStation Store grids.'),
      asset('Storefront Key Art / Background', '3840×2160 px (16:9)', 'PNG / JPG', 'Arte 4K limpa sem textos para fundo da página do produto + logo separado em PNG transparente.', 'Clean textless 4K background key art plus separate transparent PNG title logo.'),
      asset('Official Screenshots', '3840×2160 px / 1920×1080 px', 'PNG / JPG', 'Capturas nativas da plataforma sem glifos ou menções de concorrentes.', 'Platform-authentic screenshots without non-PlayStation button prompts.'),
      asset('PlayStation Store Trailer', '1920×1080 px / 3840×2160 px', 'MP4', 'Trailer em conformidade com as diretrizes de marca e classificação etária da SIE.', 'Storefront trailer compliant with SIE brand and regional age-gate rules.')
    ]
  },
  xbox: {
    label: { pt: 'Xbox / Microsoft Store', en: 'Xbox / Microsoft Store' },
    technical: [
      item(
        'Conformidade com Xbox Requirements (XRs) e Submission Validator (.msixvc)',
        'Xbox Requirements (XRs) compliance and GDK Submission Validator (.msixvc)',
        'Executar validação completa GDK sem erros críticos em Xbox Series X|S e PC.',
        'Pass GDK Submission Validator checks cleanly across Xbox Series X|S and Windows targets.'
      ),
      item(
        'Identidade Xbox Live, Connected Storage, Conquistas (1000G) e Quick Resume',
        'Xbox Live user model, Connected Storage, Achievements (1000G) and Quick Resume',
        'Testar troca de perfil, sincronização de nuvem, disparo de conquistas e ciclo PLM/Quick Resume.',
        'Test runtime profile switching, cloud storage sync, 1000G achievement unlocks, and PLM/Quick Resume.'
      )
    ],
    bureaucratic: [
      item(
        'Classificação IARC no Partner Center, Xbox Accessibility Tags e mercados/tributos',
        'Partner Center IARC ratings, Xbox Accessibility Tags and market/tax configuration',
        'Vincular ID IARC, declarar tags de acessibilidade verificadas, EULA e política de privacidade.',
        'Bind IARC rating ID, verified Xbox Accessibility Tags, EULA, and privacy policy URL.'
      )
    ],
    assets: [
      asset('Box Art (1:1 Square)', '2160×2160 px', 'PNG', 'Arte quadrada oficial (mínimo 1080×1080 px) com título legível.', 'Official 1:1 square box art (2160×2160 px preferred, 1080×1080 min) with legible title.'),
      asset('Poster Art (2:3 Portrait)', '1440×2160 px', 'PNG', 'Pôster vertical principal para navegação na Microsoft/Xbox Store e Biblioteca.', 'Primary 2:3 vertical poster tile for Xbox Store browsing and My Games & Apps.'),
      asset('Super Hero Art (16:9)', '3840×2160 px / 1920×1080 px', 'PNG', 'Arte horizontal sem texto para fundo de página do produto e destaques.', 'Clean 16:9 key art without text for PDP backdrop and featured placements.'),
      asset('Titled Hero Art & Transparent Logo', '1920×1080 px', 'PNG', 'Versão horizontal com logo posicionado na zona segura + arquivo PNG transparente do logo.', '16:9 titled hero banner within safe zone plus standalone transparent PNG logo.'),
      asset('Screenshots (4+) & Store Trailer', '3840×2160 px / 1920×1080 px', 'PNG / MP4', 'Mínimo de 4 capturas com glifos Xbox e trailer oficial.', 'Minimum 4 screenshots featuring Xbox prompts plus store trailer.')
    ]
  },
  nintendo: {
    label: { pt: 'Nintendo eShop (Switch)', en: 'Nintendo eShop (Switch)' },
    technical: [
      item(
        'Conformidade com Nintendo Lotcheck, pacote NSP e tempo de inicialização',
        'Nintendo Lotcheck compliance, NSP package verification and boot time',
        'Validar diretrizes técnicas do Nintendo Developer Portal, modo sleep, modo avião e tratamento de erros nativos.',
        'Verify Nintendo Developer Portal Lotcheck rules, sleep/wake transitions, airplane mode, and system error applets.'
      ),
      item(
        'Modos TV / Tabletop / Handheld, transições Joy-Con / Pro Controller e Save Data',
        'TV / Tabletop / Handheld modes, Joy-Con / Pro Controller transitions and Save Data',
        'Confirmar legibilidade em tela portátil, reconexão dinâmica de controles e frequência segura de escrita em NAND.',
        'Confirm handheld UI legibility, dynamic controller applet handoff, and compliant Save Data commit frequency.'
      )
    ],
    bureaucratic: [
      item(
        'Classificação IARC via Nintendo Developer Portal (e CERO se publicado no Japão)',
        'IARC rating via Nintendo Developer Portal (plus CERO if releasing in Japan)',
        'Registrar classificações regionais NOA/NOE, avisos legais e aprovação de metadados da eShop.',
        'Submit regional NOA/NOE age ratings, legal notices, and eShop metadata approval.'
      )
    ],
    assets: [
      asset('Switch Home & eShop Square Icon', '1024×1024 px', 'JPG / PNG (No Alpha)', 'Ícone quadrado RGB sem transparência e sem cantos arredondados; logo legível no menu Home do Switch.', 'Square RGB image without alpha or rounded corners; legible on Switch Home menu.'),
      asset('Nintendo eShop Hero Banner', '1920×1080 px (16:9)', 'JPG / PNG', 'Banner principal respeitando as margens seguras de sobreposição de UI da Nintendo eShop.', 'Primary 16:9 promotional banner respecting Nintendo eShop UI safe-zone margins.'),
      asset('Nintendo Switch Screenshots (6)', '1920×1080 px / 1280×720 px', 'JPG / PNG', 'Até 6 capturas reais de gameplay com glifos oficiais do Nintendo Switch.', 'Up to 6 authentic gameplay screenshots showing official Nintendo Switch button prompts.'),
      asset('Nintendo eShop Trailer', '1920×1080 px', 'MP4', 'Trailer limpo sem referências a plataformas concorrentes.', 'Clean gameplay trailer without competing platform logos or prompts.')
    ]
  },
  igdb: {
    label: { pt: 'IGDB & Diretório Twitch', en: 'IGDB & Twitch Directory' },
    technical: [
      item(
        'Vínculo de IDs externos das lojas (Steam, Epic, Consoles, Mobile) e sincronização Twitch',
        'External storefront URLs/IDs linked (Steam, Epic, Consoles, Mobile) and Twitch sync',
        'Conectar links oficiais para que a categoria do jogo apareça automaticamente e unificada na Twitch.',
        'Link official storefront URLs and IDs so the game directory entry auto-syncs cleanly to Twitch.'
      ),
      item(
        'Metadados técnicos completos (plataformas, gêneros, temas, modos e engine)',
        'Structured technical metadata (platforms, genres, themes, modes and game engine)',
        'Preencher ficha técnica estruturada, datas de lançamento por plataforma e páginas oficiais.',
        'Populate structured platforms, release dates, genres, perspectives, and game engine fields.'
      )
    ],
    bureaucratic: [
      item(
        'Rogue Fairy Studios vinculada como Developer/Publisher, créditos e classificações etárias',
        'Rogue Fairy Studios linked as Developer/Publisher, credits and official age ratings',
        'Garantir sinopse neutra dentro das diretrizes editoriais da IGDB e classificações ClassInd/ESRB/PEGI.',
        'Ensure neutral encyclopedic synopsis per IGDB guidelines plus ClassInd/ESRB/PEGI ratings.'
      )
    ],
    assets: [
      asset('IGDB Official Cover (3:4)', '1080×1440 px (min 600×800)', 'PNG / JPG (<3 MB)', 'Proporção exata 3:4 com arte oficial e logo; proibido selos etários, logos de console ou textos promocionais.', 'Strict 3:4 portrait cover with key art and logo; NO age rating badges, platform headers, or promo text (drives Twitch Box Art).'),
      asset('IGDB Screenshots (up to 10)', '1920×1080 px (16:9)', 'PNG / JPG', 'Capturas limpas de gameplay sem molduras promocionais ou marcas d’água.', 'Clean widescreen gameplay screenshots without promotional frames or watermarks.'),
      asset('Key Artworks & Transparent Logo', '1920×1080 px', 'PNG', 'Ilustrações oficiais de divulgação sem texto e logo em PNG transparente.', 'Textless official promotional key art plus transparent PNG logo.'),
      asset('Official YouTube Video ID', 'YouTube 1080p/4K', 'HTTPS URL', 'Vídeo oficial de trailer vinculado à ficha pública na IGDB.', 'Official YouTube trailer linked to the IGDB game entry.')
    ]
  },
  itch_gog: {
    label: { pt: 'GOG & Itch.io (DRM-Free)', en: 'GOG & Itch.io (DRM-Free)' },
    technical: [
      item(
        'Builds DRM-Free offline testadas e canais butler / GOG GALAXY validados',
        'Offline DRM-free builds verified and butler / GOG GALAXY channels tested',
        'Confirmar execução sem conexão de rede e atualização incremental limpa.',
        'Confirm clean offline execution without mandatory online DRM and working delta updates.'
      )
    ],
    bureaucratic: [
      item(
        'EULA offline incluso, classificação de conteúdo e cadastro fiscal das lojas',
        'Bundled offline EULA, content disclosures and store tax onboarding',
        'Revisar metadados de acessibilidade, avisos de conteúdo e perfil tributário.',
        'Verify accessibility tags, content warnings, and payout/tax setup.'
      )
    ],
    assets: [
      asset('Itch.io Cover Image', '630×500 px', 'PNG / JPG', 'Capa de grade do Itch.io com título legível.', 'Itch.io browse grid cover image with legible title.'),
      asset('GOG Vertical Cover', '1200×1680 px', 'PNG / JPG', 'Capa vertical da loja e biblioteca GOG GALAXY.', 'GOG store and GALAXY library vertical cover.'),
      asset('Store Banner & Screenshots (5+)', '1920×1080 px', 'PNG / JPG', 'Banner de cabeçalho da página e capturas de gameplay.', 'Page header banner and 5+ widescreen gameplay screenshots.')
    ]
  },
  bgg_tabletop: {
    label: { pt: 'Tabletop / Gráfica, Varejo & BGG', en: 'Tabletop / Print, Retail & BGG' },
    technical: [
      item(
        'Arquivos de pré-impressão fechados (CMYK, 300 DPI, sangria 3 mm e facas)',
        'Prepress print files finalized (CMYK, 300 DPI, 3 mm bleed and die-lines)',
        'Validar perfis de cor, margens seguras, fontes em curvas e lista completa de componentes (BOM).',
        'Verify color profiles, safe margins, outlined fonts, and complete Bill of Materials (BOM).'
      ),
      item(
        'Prova física / boneco gráfico aprovado antes da tiragem final',
        'Physical white-box / press proof approved prior to mass production',
        'Conferir gramatura de cartas/tabuleiro, encaixe de berço (insert) e legibilidade física.',
        'Inspect cardstock weight, punchboard tolerances, insert fit, and physical legibility.'
      )
    ],
    bureaucratic: [
      item(
        'Código EAN-13 / ISBN, selo INMETRO / CE / ASTM e ficha no BoardGameGeek / Ludopedia',
        'EAN-13 / ISBN barcode, INMETRO / CE / ASTM safety marks and BGG / Ludopedia entry',
        'Validar marcações legais obrigatórias na tampa/fundo da caixa e cadastro oficial de créditos.',
        'Verify mandatory safety certifications and barcode on box bottom plus official BGG/Ludopedia listing.'
      )
    ],
    assets: [
      asset('3D Box Render & Front Cover (BGG)', '1500×1500 px & 1200×1600 px', 'PNG / JPG', 'Render 3D da caixa e arte frontal para BoardGameGeek, Ludopedia e catálogos de distribuição.', '3D box shot and flat front cover for BoardGameGeek, Ludopedia, and distributor catalogs.'),
      asset('Tabletop Component Photos / Renders (4+)', '1920×1080 px', 'PNG / JPG', 'Fotos ou renders do jogo montado em mesa exibindo tabuleiro, cartas e peças.', 'Photos or renders of the full tabletop setup showcasing board, cards, and components.'),
      asset('Rulebook PDF & Retail Sell Sheet', 'A4 / Letter PDF', 'PDF', 'Manual de regras final revisado e folha comercial de 1 página para lojistas.', 'Final proofread digital rulebook PDF and 1-page distributor/retail sell sheet.')
    ]
  },
  custom: {
    label: { pt: 'Plataforma Customizada', en: 'Custom Platform' },
    technical: [],
    bureaucratic: [],
    assets: []
  }
}

function buildPreset (storefront, makeId, locale = 'pt') {
  const lang = String(locale || 'pt').toLowerCase().startsWith('pt') ? 'pt' : 'en'
  const entry = catalog[storefront] || catalog.custom
  const checklist = []
  for (const category of checkCategories) {
    for (const row of entry[category] || []) {
      checklist.push({
        id: makeId(),
        category,
        title: lang === 'pt' ? row.ptTitle : row.enTitle,
        spec: lang === 'pt' ? row.ptSpec : row.enSpec,
        required: row.required !== false,
        status: 'pending',
        waiverReason: '',
        assigneeId: null,
        documentId: null,
        taskId: null,
        notes: '',
        verifiedBy: null,
        verifiedAt: null
      })
    }
  }
  const assets = (entry.assets || []).map(slot => ({
    id: makeId(),
    name: slot.name,
    dimensions: slot.dimensions,
    format: slot.format,
    spec: lang === 'pt' ? slot.ptSpec : slot.enSpec,
    required: slot.required !== false,
    status: 'pending',
    waiverReason: '',
    artifactVersionId: null,
    uri: null,
    checksum: null,
    toolUri: null,
    locale: 'all',
    notes: '',
    verifiedBy: null,
    verifiedAt: null
  }))
  return {
    defaultName: entry.label[lang],
    checklist,
    assets
  }
}

function evaluateTargetReadiness (target) {
  const activeChecks = (target.checklist || []).filter(c => c.required !== false)
  const readyChecks = activeChecks.filter(c => c.status === 'done' || (c.status === 'na' && c.waiverReason))
  const missingChecks = activeChecks.filter(c => !(c.status === 'done' || (c.status === 'na' && c.waiverReason)))

  const activeAssets = (target.assets || []).filter(a => a.required !== false)
  const readyAssets = activeAssets.filter(a => (a.status === 'done' && (a.artifactVersionId || a.uri)) || (a.status === 'na' && a.waiverReason))
  const missingAssets = activeAssets.filter(a => !((a.status === 'done' && (a.artifactVersionId || a.uri)) || (a.status === 'na' && a.waiverReason)))

  return {
    ready: missingChecks.length === 0 && missingAssets.length === 0,
    checksDone: readyChecks.length,
    checksTotal: activeChecks.length,
    assetsDone: readyAssets.length,
    assetsTotal: activeAssets.length,
    missingChecks,
    missingAssets
  }
}

module.exports = {
  storefronts,
  submissionStatuses,
  itemStatuses,
  checkCategories,
  catalog,
  buildPreset,
  evaluateTargetReadiness
}

