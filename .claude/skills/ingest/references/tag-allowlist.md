# Tag allowlist (release-pass + artist rollup)

The durable allowlist for Bandcamp keyword → genre/locale tags. Extend as keywords emerge from drop-counter reports.

```python
# normalize variants -> canonical BEFORE matching
NORM = {"post punk":"post-punk","powerpop":"power pop","power-pop":"power pop",
 "rock n roll":"rock'n'roll","rock and roll":"rock'n'roll","rock n' roll":"rock'n'roll",
 "lofi":"lo-fi","altpop":"alt-pop","hip hop":"hip-hop","synthpop":"synth pop",
 "art-punk":"art punk","proto punk":"proto-punk","garage-rock":"garage rock"}
GENRES = {
 "punk","post-punk","hardcore punk","garage punk","hardcore","garage","garage rock",
 "coldwave","new wave","no wave","synthwave","synthpop","synth pop","synth-punk","egg punk",
 "art punk","power pop","proto-punk","oi","street punk","d-beat","crust","post-hardcore",
 "goth","gothic","darkwave","minimal synth","ebm","industrial","noise rock","noise","drone",
 "psychedelic","psych","psych rock","psychedelic rock","electronic","experimental","ambient",
 "folk","gospel","funk","disco","avant-garde","hip-hop","jazz","metal","alternative","rock",
 "rock & roll","rock'n'roll","pop","surf","dub","krautrock","shoegaze","dream pop","country",
 "soul","blues","indie","indie rock","lo-fi","dance","techno","house","improv","free jazz",
 "synth","glam","britpop","punk rock","art rock","skate punk","alt-pop","alt-psych",
 "dark ambient","tape music","musique concrete","synthesizer","e.b.m","dancepunk",
 "vaporwave","harsh noise","plunderphonics","deathrock","post-industrial",
 "power electronics","neofolk","minimal wave","ambient electronic","soundscape"}
LOCALES = {
 "australia":"Australian","netherlands":"Dutch","finland":"Finnish","uk":"British",
 "england":"British","canada":"Canadian","germany":"German","france":"French","italy":"Italian",
 "spain":"Spanish","japan":"Japanese","ireland":"Irish",
 "cincinnati":"Cincinnati","richmond":"Richmond","detroit":"Detroit","cleveland":"Cleveland",
 "melbourne":"Melbourne","minneapolis":"Minneapolis","seattle":"Seattle","las vegas":"Las Vegas",
 "bloomington":"Bloomington","chicago":"Chicago","kansas city":"Kansas City","memphis":"Memphis",
 "kyiv":"Kyiv","baltimore":"Baltimore",
 "california":"California","ohio":"Ohio","brooklyn":"Brooklyn",
 "los angeles":"Los Angeles","san francisco":"San Francisco",
 "arizona":"Arizona","austin":"Austin","oberlin":"Oberlin"}
# Next-tier candidates (promote on demand): kiwi pop, grungepop, noise-pop, indie pop;
# cities London/Olympia/Charlottesville/Leipzig.
```

### Promotion loop

1. Cache raw keywords per release (`/tmp/releases-raw.json`)
2. Report top non-allowlist keywords by frequency
3. Promote high-value genres/cities (ask user); drop noise
4. Rebuild batch from cache with expanded allowlist
