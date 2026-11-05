# naturalearth testdata

land-excerpt.geojson is three features of Natural Earth's 1:50m land
file, ne_50m_land.geojson, as fetched on 2026-11-02 from
https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_50m_land.geojson
(1636166 bytes, sha256
e874b27a51d146452be360cafb3cc50c86001074a67d534113e6534682f9826b).
Natural Earth is in the public domain.

The features, by their place in the file's list (counting from 0):

- 5, a polygon of five points: an islet in the Florida Keys. Unchanged.
- 657, the file's one multipolygon: two polygons off the Russian coast,
  at about 53 east and 71 north. Unchanged.
- 1380, Afro-Eurasia: a polygon of 10297 points with one hole, the
  Caspian Sea, of 391 points. It is the file's one polygon with a hole.

Feature 1380 would be 250 kilobytes whole, so it is thinned. Its outline
keeps every 50th point and its hole every 4th, and each keeps its last
point so that it still closes. The outline then has 207 points and the
hole 99. The feature's bbox member is dropped, since it is no longer the
box of the points. The test places are well clear of these coasts, so
the thinning does not move them. The same places were asked of the
whole file, and it answered the same.

It was cut with jq, from the real file in a scratch folder:

    jq -c '
    def thin($n): . as $r | [range(0; length)
      | select(. % $n == 0 or . == ($r | length - 1)) | $r[.]];
    {type, name, crs, features: [
      .features[5], .features[657],
      (.features[1380] | del(.bbox) | .geometry.coordinates
        |= [(.[0] | thin(50)), (.[1] | thin(4))])]}' ne_50m_land.geojson
