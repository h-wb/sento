package store

import "context"

type ReviewItem struct {
	Status      string `json:"status"`
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	SeriesID    int64  `json:"seriesId"`
	SeriesTitle string `json:"seriesTitle"`
	ItemID      int64  `json:"itemId"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
}

func (s *Store) ReviewItems(ctx context.Context, sid int64) ([]ReviewItem, error) {
	return list[ReviewItem](ctx, s, `
 SELECT 'file-'||CAST(f.id AS TEXT) AS id,'file' AS kind,f.series_id,s.title AS series_title,f.id AS item_id,f.path AS title,
 CASE WHEN f.presence='unavailable' THEN 'Storage unavailable: '||f.presence_error WHEN f.presence='unknown' THEN 'File presence has not been checked' ELSE 'Confirm the edition and books in this file' END AS detail, f.presence AS status
 FROM files f JOIN series s ON s.id=f.series_id WHERE (?=0 OR s.id=?) AND (f.needs_review=TRUE OR f.unrequested=TRUE OR f.presence IN('unavailable','unknown') OR EXISTS(SELECT 1 FROM asset_contents c WHERE c.file_id=f.id AND c.verified=FALSE))
 UNION ALL
 SELECT 'queue-'||CAST(q.id AS TEXT),'download',q.series_id,s.title,q.id,q.title,COALESCE(q.error,''),q.status
 FROM queue_items q JOIN series s ON s.id=q.series_id WHERE (?=0 OR s.id=?) AND q.status IN('needs_review','dispatch_unknown','failed','retry_wait')
 UNION ALL
 SELECT 'chapter-'||CAST(o.id AS TEXT),'chapter',c.series_id,s.title,o.id,o.label,'New chapter from '||o.source,o.state
 FROM unit_observations o JOIN collections c ON c.id=o.collection_id JOIN series s ON s.id=c.series_id WHERE (?=0 OR s.id=?) AND o.state='pending'
 ORDER BY series_title,kind,item_id LIMIT 500`, sid, sid, sid, sid, sid, sid)
}
