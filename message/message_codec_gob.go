package message

import (
	"bytes"
	"encoding/gob"
)

type MessageCodecGob struct {
}

func (MessageCodecGob) EncodePingMessage(descriptorHeader DescriptorHeader) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(descriptorHeader)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (MessageCodecGob) EncodePongMessage(descriptorHeader DescriptorHeader, pongMessage PongMessage) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(descriptorHeader)
	if err != nil {
		return nil, err
	}

	err = encoder.Encode(pongMessage)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (MessageCodecGob) EncodeQueryMessage(descriptorHeader DescriptorHeader, queryMessage QueryMessage) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(descriptorHeader)
	if err != nil {
		return nil, err
	}

	err = encoder.Encode(queryMessage)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (MessageCodecGob) EncodeQueryHitMessage(descriptorHeader DescriptorHeader, queryHitMessage QueryHitMessage) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(descriptorHeader)
	if err != nil {
		return nil, err
	}

	err = encoder.Encode(queryHitMessage)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (MessageCodecGob) DecodeDescriptorHeader(buffer *bytes.Buffer) (DescriptorHeader, error) {
	decoder := gob.NewDecoder(buffer)
	var descriptorHeader DescriptorHeader

	err := decoder.Decode(&descriptorHeader)
	if err != nil {
		return descriptorHeader, err
	}

	return descriptorHeader, nil
}

func (MessageCodecGob) DecodePongMessage(buffer *bytes.Buffer) (PongMessage, error) {
	decoder := gob.NewDecoder(buffer)

	var pongMessage PongMessage
	err := decoder.Decode(&pongMessage)
	if err != nil {
		return pongMessage, err
	}

	return pongMessage, nil
}

func (MessageCodecGob) DecodeQueryMessage(buffer *bytes.Buffer) (QueryMessage, error) {
	decoder := gob.NewDecoder(buffer)

	var queryMessage QueryMessage
	err := decoder.Decode(&queryMessage)
	if err != nil {
		return queryMessage, err
	}

	return queryMessage, nil
}

func (MessageCodecGob) DecodeQueryHitMessage(buffer *bytes.Buffer) (QueryHitMessage, error) {
	decoder := gob.NewDecoder(buffer)

	var queryHitMessage QueryHitMessage
	err := decoder.Decode(&queryHitMessage)
	if err != nil {
		return queryHitMessage, err
	}

	return queryHitMessage, nil
}
