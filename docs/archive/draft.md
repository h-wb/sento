
Only one source of truth for metadata and volumes/chapters structure: Mangabaka

Should use the concept of mangabaka of collections/work to choose a manga based on a edition 

Should it be:
- Choose 1 manga then choose it’s edition OR
- Choose an edition of a manga and it’s a separate entity
- maybe people wants to have multiple editions
- volumes/chapters number is then calculated from the edition

Downloads should support Torrent and the tachiyomi modules 

I have to see how we want to build that but suwayomi already made so server code to get the tachiyomi extension. I don’t know if we want to ship that as part of the app because it’s java, i also don’t want to rewrite the whole things because it would be a lot of work but i donn’t my backend to be in java 

maybe a solution is to have a “micro service” where we ship the module extension separately and we can then query to it like we would query Prowlarr for the torrent

We need a lot of files processing utilities 
need to be able to move files with some hardlink etc etc but we can also move directly because it’s not like torrent where we need to seed. But hard linking the tachiyomi downloaded manga would just be a way to keep the tachiyomi module aware that we already have this file, no need to look for it or something. Probably a hash thing 

we should be able to detect what file is this based on the numbering, ch1, 1 or v1. It’s gonna depends on we track chapters/volumes. Some sites don’t include chapters or volumes in their naming if they only ship chapters we should be able to detect/configure that

when looking for a chapter, we should be able to find a chapter but when we look for volume we should be able to detect all his chapters as well. This might be really tricky because we don’t necessarily know the number of chapters for a volume. I don’t know if mangabaka gives that

when looking for a volume we should be able to detect pack as well like Vol1-Vol38
we should build a really solid parser with regression testing on extension 

post processing is a big part of it. I don’t know if we should consider writing the Comicinfo.xml as part of this tool but we should include all info in the file name at least. That would kind of be a new standard but that could allow client to have unified way like : “One Piece (vol:1)(edition:paperback)(mangabaka_id:123)(lang:en)(team:DarkHorse)” something like that. but yeah that could also be technically something we write to the xml file.

The structure between volumes and chapters should be clear. A lot of people are downloading chapters but i prefer to download high quality volume. Should it 

Build in go for performance ?
shadcn for the fe, classic in 2026 it seems

# Resources

- suwayomi: /Users/hugo/Repositories/Suwayomi-Server
- mangabaka: https://mangabaka.org/