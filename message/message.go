package message

const PING_DESCRIPTOR = 0x00
const PONG_DESCRIPTOR = 0x01
const QUERY_DESCRIPTOR = 0x80
const QUERY_HIT_DESCRIPTOR = 0x81

type DescriptorHeader struct {
	DescriptorID      [16]byte
	PayloadDescriptor uint8
	TTL               uint8
	Hops              uint8
}

type PongMessage struct {
	Port                    uint16
	IPAddress               [4]byte
	NumberOfSharedFiles     uint32
	NumberOfKilobytesShared uint32
}

type QueryMessage struct {
	MinimumSpeed   uint16
	SearchCriteria string
}

type QueryHitMessage struct {
	NumberOfHits uint8
	Port         uint16
	IPAddress    [4]byte
	Speed        uint32
	ResultSet    []QueryResult
}

type QueryResult struct {
	//Missing file index
	FileSize uint32
	FileName string
}
