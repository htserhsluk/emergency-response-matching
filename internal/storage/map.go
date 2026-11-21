package storage

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
)

func (s *Store) MapLocations(ctx context.Context, kind, after string) (contracts.MapPage, error) {
	out := contracts.MapPage{Items: []contracts.MapLocation{}}
	if !contracts.ValidMapQuery(kind, after) {
		return out, contracts.ErrInvalid
	}
	rows, err := s.pool.Query(ctx, `SELECT kind,id,type,status,ST_Y(location::geometry),ST_X(location::geometry),related FROM (
 SELECT 'request' AS kind,id,type,status,location,coalesce(assigned_resource_id,'') AS related FROM requests WHERE ($1='ALL' OR type=$1) AND ('request:'||id)>$2
 UNION ALL SELECT 'resource',id,type,status,location,coalesce(assigned_request_id,'') FROM resources WHERE ($1='ALL' OR type=$1) AND ('resource:'||id)>$2
 ) locations ORDER BY kind,id LIMIT $3`, kind, after, contracts.MapPageSize+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var p contracts.MapLocation
		var lat, lon float64
		var related string
		if err := rows.Scan(&p.Kind, &p.ID, &p.Type, &p.Status, &lat, &lon, &related); err != nil {
			return out, err
		}
		p.Location = &contracts.Location{Latitude: &lat, Longitude: &lon}
		if p.Kind == "request" {
			p.AssignedResourceID = related
		} else {
			p.AssignedRequestID = related
		}
		out.Items = append(out.Items, p)
	}
	return out, rows.Err()
}

func (s *Store) NearbyRequests(ctx context.Context, q contracts.NearbyQuery) ([]contracts.NearbyRequest, error) {
	if q.Validate() != nil {
		return nil, contracts.ErrInvalid
	}
	rows, err := s.pool.Query(ctx, `WITH point AS (SELECT ST_SetSRID(ST_MakePoint($1,$2),4326)::geography AS origin)
 SELECT r.id,ST_Y(r.location::geometry),ST_X(r.location::geometry),ST_Distance(r.location,point.origin) AS distance
 FROM requests r CROSS JOIN point WHERE r.type=$3 AND r.status IN ('PENDING','UNMATCHED','ASSIGNING')
 AND NOT EXISTS (SELECT 1 FROM assignment_attempts a WHERE a.request_id=r.id AND a.phase IN ('INTENT','RESERVED','FINALIZED'))
 AND NOT EXISTS (SELECT 1 FROM processed_events e WHERE e.request_id=r.id AND e.generation=r.generation AND e.state='PAUSED')
 AND ST_DWithin(r.location,point.origin,$4) ORDER BY distance,r.id LIMIT 5`, *q.Location.Longitude, *q.Location.Latitude, q.Type, contracts.MaxMatchDistanceMeters)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.NearbyRequest{}
	for rows.Next() {
		var item contracts.NearbyRequest
		var lat, lon float64
		if err := rows.Scan(&item.ID, &lat, &lon, &item.DistanceMeters); err != nil {
			return nil, err
		}
		item.Location = &contracts.Location{Latitude: &lat, Longitude: &lon}
		out = append(out, item)
	}
	return out, rows.Err()
}
