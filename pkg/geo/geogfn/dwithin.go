// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package geogfn

import (
	"github.com/cockroachdb/cockroach/pkg/geo"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/golang/geo/s1"
)

// DWithin returns whether a is within distance d of b. If A or B contains empty
// Geography objects, this will return false. If inclusive, DWithin is
// equivalent to Distance(a, b) <= d. Otherwise, DWithin is instead equivalent
// to Distance(a, b) < d.
func DWithin(
	a geo.Geography,
	b geo.Geography,
	distance float64,
	useSphereOrSpheroid UseSphereOrSpheroid,
	exclusivity geo.FnExclusivity,
) (bool, error) {
	if a.SRID() != b.SRID() {
		return false, geo.NewMismatchingSRIDsError(a.SpatialObject(), b.SpatialObject())
	}
	if distance < 0 {
		return false, pgerror.Newf(pgcode.InvalidParameterValue, "dwithin distance cannot be less than zero")
	}
	spheroid, err := spheroidFromGeography(a)
	if err != nil {
		return false, err
	}

	angleToExpand := s1.Angle(distance / spheroid.SphereRadius())
	if useSphereOrSpheroid == UseSpheroid {
		angleToExpand *= (1 + SpheroidErrorFraction)
	}
	if !a.BoundingCap().Expanded(angleToExpand).Intersects(b.BoundingCap()) {
		return false, nil
	}

	aRegions, err := a.AsS2(geo.EmptyBehaviorError)
	if err != nil {
		if geo.IsEmptyGeometryError(err) {
			return false, nil
		}
		return false, err
	}
	bRegions, err := b.AsS2(geo.EmptyBehaviorError)
	if err != nil {
		if geo.IsEmptyGeometryError(err) {
			return false, nil
		}
		return false, err
	}
	maybeClosestDistance, err := distanceGeographyRegions(
		spheroid,
		useSphereOrSpheroid,
		aRegions,
		bRegions,
		a.BoundingRect().Intersects(b.BoundingRect()),
		distance,
		exclusivity,
	)
	if err != nil {
		return false, err
	}
	if exclusivity == geo.FnExclusive {
		return maybeClosestDistance < distance, nil
	}
	return maybeClosestDistance <= distance, nil
}
