package message

import (
	"bytes"
	"encoding/binary"
)

type MessageCodecBasic struct {
}

func Reverse(src []byte) []byte {
	var res []byte
	for i := len(src) - 1; i >= 0; i-- {
		res = append(res, src[i])
	}

	return res
}

func (MessageCodecBasic) EncodePingMessage(descriptorHeader DescriptorHeader) ([]byte, error) {
	var buffer bytes.Buffer
	err := binary.Write(&buffer, binary.LittleEndian, descriptorHeader)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (MessageCodecBasic) EncodePongMessage(descriptorHeader DescriptorHeader, pongMessage PongMessage) ([]byte, error) {
	var buffer bytes.Buffer
	err := binary.Write(&buffer, binary.LittleEndian, descriptorHeader)
	if err != nil {
		return nil, err
	}

	err = binary.Write(&buffer, binary.LittleEndian, pongMessage)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (MessageCodecBasic) EncodeQueryMessage(descriptorHeader DescriptorHeader, queryMessage QueryMessage) ([]byte, error) {
	var descriptorHeaderBuffer bytes.Buffer
	err := binary.Write(&descriptorHeaderBuffer, binary.LittleEndian, descriptorHeader)
	if err != nil {
		return nil, err
	}

	queryMessageData := make([]byte, 2)
	binary.LittleEndian.PutUint16(queryMessageData, queryMessage.MinimumSpeed)
	queryMessageData = append(queryMessageData, Reverse([]byte(queryMessage.SearchCriteria))...)
	queryMessageData = append(queryMessageData, 0x00)

	return append(descriptorHeaderBuffer.Bytes(), queryMessageData...), nil
}

func (m MessageCodecBasic) EncodeQueryHitMessage(descriptorHeader DescriptorHeader, queryHitMessage QueryHitMessage) ([]byte, error) {
	var descriptorHeaderBuffer bytes.Buffer
	err := binary.Write(&descriptorHeaderBuffer, binary.LittleEndian, descriptorHeader)
	if err != nil {
		return nil, err
	}

	var queryHitMessageBuffer bytes.Buffer
	binary.Write(&queryHitMessageBuffer, binary.LittleEndian, queryHitMessage.NumberOfHits)
	binary.Write(&queryHitMessageBuffer, binary.LittleEndian, queryHitMessage.Port)
	binary.Write(&queryHitMessageBuffer, binary.LittleEndian, queryHitMessage.IPAddress)
	binary.Write(&queryHitMessageBuffer, binary.LittleEndian, queryHitMessage.Speed)

	for _, queryResult := range queryHitMessage.ResultSet {
		fileSizeBuffer := make([]byte, 4) // hardcoded
		binary.LittleEndian.PutUint32(fileSizeBuffer, queryResult.FileSize)
		queryHitMessageBuffer.Write(fileSizeBuffer)
		queryHitMessageBuffer.Write(Reverse([]byte(queryResult.FileName)))
		queryHitMessageBuffer.Write([]byte{0x00, 0x00})
	}

	return append(descriptorHeaderBuffer.Bytes(), queryHitMessageBuffer.Bytes()...), nil
}

func (MessageCodecBasic) DecodeDescriptorHeader(buffer *bytes.Buffer) (DescriptorHeader, error) {
	var descriptorHeader DescriptorHeader

	err := binary.Read(buffer, binary.LittleEndian, &descriptorHeader)
	if err != nil {
		return descriptorHeader, err
	}

	return descriptorHeader, nil
}

func (MessageCodecBasic) DecodePongMessage(buffer *bytes.Buffer) (PongMessage, error) {
	var pongMessage PongMessage

	err := binary.Read(buffer, binary.LittleEndian, &pongMessage)
	if err != nil {
		return pongMessage, err
	}

	return pongMessage, nil
}

func (MessageCodecBasic) DecodeQueryMessage(buffer *bytes.Buffer) (QueryMessage, error) {
	var queryMessage QueryMessage

	var minimumSpeed uint16
	err := binary.Read(buffer, binary.LittleEndian, &minimumSpeed)
	if err != nil {
		return queryMessage, err
	}

	searchCriteriaData, err := buffer.ReadBytes(0x00)
	if err != nil {
		return queryMessage, err
	}

	queryMessage = QueryMessage{
		MinimumSpeed:   minimumSpeed,
		SearchCriteria: string(Reverse(searchCriteriaData[:len(searchCriteriaData)-1])),
	}

	return queryMessage, nil
}

func (MessageCodecBasic) DecodeQueryHitMessage(buffer *bytes.Buffer) (QueryHitMessage, error) {
	var queryHitMessage QueryHitMessage

	var numberOfHits uint8
	err := binary.Read(buffer, binary.LittleEndian, &numberOfHits)
	if err != nil {
		return queryHitMessage, err
	}

	var port uint16
	err = binary.Read(buffer, binary.LittleEndian, &port)
	if err != nil {
		return queryHitMessage, err
	}

	var ipAddress [4]byte
	err = binary.Read(buffer, binary.LittleEndian, &ipAddress)
	if err != nil {
		return queryHitMessage, err
	}

	var speed uint32
	err = binary.Read(buffer, binary.LittleEndian, &speed)
	if err != nil {
		return queryHitMessage, err
	}

	var resultSet []QueryResult
	for i := uint8(0); i < numberOfHits; i++ {
		var queryResult QueryResult
		err = binary.Read(buffer, binary.LittleEndian, &queryResult.FileSize)
		if err != nil {
			return queryHitMessage, err
		}

		var fileNameData []byte
		for {
			b, err := buffer.ReadByte()
			if err != nil {
				return queryHitMessage, err
			}

			fileNameData = append(fileNameData, b)
			fileNameLen := len(fileNameData)
			if fileNameData[fileNameLen-1] == 0x00 &&
				fileNameLen > 1 && fileNameData[fileNameLen-2] == 0x00 {
				break
			}
		}

		queryResult.FileName = string(Reverse(fileNameData[:len(fileNameData)-2]))

		resultSet = append(resultSet, queryResult)
	}

	queryHitMessage = QueryHitMessage{
		NumberOfHits: numberOfHits,
		Port:         port,
		IPAddress:    ipAddress,
		Speed:        speed,
		ResultSet:    resultSet,
	}

	return queryHitMessage, nil
}
