package radigo

import (
	"github.com/yyoshiki41/go-radiko"
	"log"
	"time"
)

type Tag struct {
	Title   string
	Artist  string
	PubDate time.Time
}

func CreateTagFromPg(pg *radiko.Prog, station string) Tag {
	start, err := time.Parse("20060102150405", pg.Ft)
	if err != nil {
		log.Println("cannot parse date", pg.Ft)
		start = time.Unix(0, 0)
	}
	return Tag{pg.Title, station, start}
}

func (t Tag) toFfMpegOption() []string {
	metadata := []string{
		"-metadata", "title=" + t.Title + " " + t.PubDate.Format("2006年01月02日"),
		"-metadata", "genre=Radio",
		"-metadata", "artist=" + t.Artist,
	}
	return metadata
}

func (t Tag) toFileName() string {
	return t.Title + " " + t.PubDate.Format("2006年01月02日")
}
