// Go MySQL Driver - A MySQL-Driver for Go's database/sql package
//
// Copyright 2012 The Go-MySQL-Driver Authors. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package mysql

import (
	"bytes"
	"crypto/tls"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"time"
)

// MySQL client/server protocol documentations.
// https://dev.mysql.com/doc/dev/mysql-server/latest/PAGE_PROTOCOL.html
// https://mariadb.com/kb/en/clientserver-protocol/

// read n bytes from mc.buf
func (mc *mysqlConn) readNext(n int) ([]byte, error) {
	if mc.buf.len() < n {
		err := mc.buf.fill(n, mc.readWithTimeout)
		if err != nil {
			return nil, err
		}
	}
	return mc.buf.readNext(n), nil
}

// Read packet to buffer 'data'
func (mc *mysqlConn) readPacket() ([]byte, error) {
	var prevData []byte
	invalidSequence := false

	readNext := mc.readNext
	if mc.compress {
		readNext = mc.compIO.readNext
	}

	for {
		// read packet header
		data, err := readNext(4)
		if err != nil {
			mc.close()
			if cerr := mc.canceled.Value(); cerr != nil {
				return nil, cerr
			}
			mc.log(err)
			return nil, ErrInvalidConn
		}

		// packet length [24 bit]
		pktLen := getUint24(data[:3])
		seq := data[3]

		// check packet sync [8 bit]
		if seq != mc.sequence {
			mc.log(fmt.Sprintf("[warn] unexpected sequence nr: expected %v, got %v", mc.sequence, seq))
			// MySQL and MariaDB doesn't check packet nr in compressed packet.
			if !mc.compress {
				// For large packets, we stop reading as soon as sync error.
				if len(prevData) > 0 {
					mc.close()
					return nil, ErrPktSyncMul
				}
				invalidSequence = true
			}
		}
		mc.sequence = seq + 1

		// packets with length 0 terminate a previous packet which is a
		// multiple of (2^24)-1 bytes long
		if pktLen == 0 {
			// there was no previous packet
			if prevData == nil {
				mc.log(ErrMalformPkt)
				mc.close()
				return nil, ErrInvalidConn
			}
			return prevData, nil
		}

		// read packet body [pktLen bytes]
		data, err = readNext(pktLen)
		if err != nil {
			mc.close()
			if cerr := mc.canceled.Value(); cerr != nil {
				return nil, cerr
			}
			mc.log(err)
			return nil, ErrInvalidConn
		}

		// return data if this was the last packet
		if pktLen < maxPacketSize {
			// zero allocations for non-split packets
			if prevData != nil {
				data = append(prevData, data...)
			}
			if invalidSequence {
				mc.close()
				// return sync error only for regular packet.
				// error packets may have wrong sequence number.
				if data[0] != iERR {
					return nil, ErrPktSync
				}
			}
			return data, nil
		}

		prevData = append(prevData, data...)
	}
}

// Write packet buffer 'data'
func (mc *mysqlConn) writePacket(data []byte) error {
	pktLen := len(data) - 4
	if pktLen > mc.maxAllowedPacket {
		return ErrPktTooLarge
	}

	writeFunc := mc.writeWithTimeout
	if mc.compress {
		writeFunc = mc.compIO.writePackets
	}

	for {
		size := min(maxPacketSize, pktLen)
		putUint24(data[:3], size)
		data[3] = mc.sequence

		// Write packet
		if debug {
			fmt.Fprintf(os.Stderr, "writePacket: size=%v seq=%v\n", size, mc.sequence)
		}

		n, err := writeFunc(data[:4+size])
		if err != nil {
			mc.cleanup()
			if cerr := mc.canceled.Value(); cerr != nil {
				return cerr
			}
			if n == 0 && pktLen == len(data)-4 {
				// only for the first loop iteration when nothing was written yet
				mc.log(err)
				return errBadConnNoWrite
			} else {
				return err
			}
		}
		if n != 4+size {
			// io.Writer(b) must return a non-nil error if it cannot write len(b) bytes.
			// The io.ErrShortWrite error is used to indicate that this rule has not been followed.
			mc.cleanup()
			return io.ErrShortWrite
		}

		mc.sequence++
		if size != maxPacketSize {
			return nil
		}
		pktLen -= size
		data = data[size:]
	}
}

/******************************************************************************
*                           Initialization Process                            *
******************************************************************************/

// Handshake Initialization Packet
// http://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::Handshake
func (mc *mysqlConn) readHandshakePacket() (data []byte, plugin string, err error) {
	data, err = mc.readPacket()
	if err != nil {
		return
	}

	if data[0] == iERR {
		return nil, "", mc.handleErrorPacket(data)
	}

	// protocol version [1 byte]
	if data[0] < minProtocolVersion {
		return nil, "", fmt.Errorf(
			"unsupported protocol version %d. Version %d or higher is required",
			data[0],
			minProtocolVersion,
		)
	}

	// server version [null terminated string]
	// connection id [4 bytes]
	pos := 1 + bytes.IndexByte(data[1:], 0x00) + 1 + 4

	// first part of the password cipher [8 bytes]
	authData := data[pos : pos+8]

	// (filler) always 0x00 [1 byte]
	pos += 8 + 1

	// capability flags (lower 2 bytes) [2 bytes]
	mc.flags = clientFlag(binary.LittleEndian.Uint16(data[pos : pos+2]))
	if mc.flags&clientProtocol41 == 0 {
		return nil, "", ErrOldProtocol
	}
	if mc.flags&clientSSL == 0 && mc.cfg.TLS != nil {
		if mc.cfg.AllowFallbackToPlaintext {
			mc.cfg.TLS = nil
		} else {
			return nil, "", ErrNoTLS
		}
	}
	pos += 2

	if len(data) > pos {
		// character set [1 byte]
		// status flags [2 bytes]
		pos += 3
		// capability flags (upper 2 bytes) [2 bytes]
		mc.flags |= clientFlag(binary.LittleEndian.Uint16(data[pos:pos+2])) << 16
		pos += 2
		// length of auth-plugin-data [1 byte]
		// reserved (all [00]) [10 bytes]
		pos += 11

		// second part of the password cipher [minimum 13 bytes],
		// where len=MAX(13, length of auth-plugin-data - 8)
		//
		// The web documentation is ambiguous about the length. However,
		// according to mysql-5.7/sql/auth/sql_authentication.cc line 538,
		// the 13th byte is "\0 byte, terminating the second part of
		// a scramble". So the second part of the password cipher is
		// a NULL terminated string that's at least 13 bytes with the
		// last byte being NULL.
		//
		// The official Python library uses the fixed length 12
		// which seems to work but technically could have a hidden bug.
		authData = append(authData, data[pos:pos+12]...)
		pos += 13

		// EOF if version (>= 5.5.7 and < 5.5.10) or (>= 5.6.0 and < 5.6.2)
		// \NUL otherwise
		if end := bytes.IndexByte(data[pos:], 0x00); end != -1 {
			plugin = string(data[pos : pos+end])
		} else {
			plugin = string(data[pos:])
		}

		// make a memory safe copy of the cipher slice
		var b [20]byte
		copy(b[:], authData)
		return b[:], plugin, nil
	}

	// make a memory safe copy of the cipher slice
	var b [8]byte
	copy(b[:], authData)
	return b[:], plugin, nil
}

// Client Authentication Packet
// http://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::HandshakeResponse
func (mc *mysqlConn) writeHandshakeResponsePacket(authResp []byte, plugin string) error {
	// Adjust client flags based on server support
	clientFlags := clientProtocol41 |
		clientSecureConn |
		clientLongPassword |
		clientTransactions |
		clientLocalFiles |
		clientPluginAuth |
		clientMultiResults |
		mc.flags&clientConnectAttrs |
		mc.flags&clientLongFlag

	sendConnectAttrs := mc.flags&clientConnectAttrs != 0

	if mc.cfg.ClientFoundRows {
		clientFlags |= clientFoundRows
	}
	if mc.cfg.compress && mc.flags&clientCompress == clientCompress {
		clientFlags |= clientCompress
	}
	// To enable TLS / SSL
	if mc.cfg.TLS != nil {
		clientFlags |= clientSSL
	}

	if mc.cfg.MultiStatements {
		clientFlags |= clientMultiStatements
	}

	// encode length of the auth plugin data
	var authRespLEIBuf [9]byte
	authRespLen := len(authResp)
	authRespLEI := appendLengthEncodedInteger(authRespLEIBuf[:0], uint64(authRespLen))
	if len(authRespLEI) > 1 {
		// if the length can not be written in 1 byte, it must be written as a
		// length encoded integer
		clientFlags |= clientPluginAuthLenEncClientData
	}

	pktLen := 4 + 4 + 1 + 23 + len(mc.cfg.User) + 1 + len(authRespLEI) + len(authResp) + 21 + 1

	// To specify a db name
	if n := len(mc.cfg.DBName); n > 0 {
		clientFlags |= clientConnectWithDB
		pktLen += n + 1
	}

	// encode length of the connection attributes
	var connAttrsLEI []byte
	if sendConnectAttrs {
		var connAttrsLEIBuf [9]byte
		connAttrsLen := len(mc.connector.encodedAttributes)
		connAttrsLEI = appendLengthEncodedInteger(connAttrsLEIBuf[:0], uint64(connAttrsLen))
		pktLen += len(connAttrsLEI) + len(mc.connector.encodedAttributes)
	}

	// Calculate packet length and get buffer with that size
	data, err := mc.buf.takeBuffer(pktLen + 4)
	if err != nil {
		mc.cleanup()
		return err
	}

	// ClientFlags [32 bit]
	binary.LittleEndian.PutUint32(data[4:], uint32(clientFlags))

	// MaxPacketSize [32 bit] (none)
	binary.LittleEndian.PutUint32(data[8:], 0)

	// Collation ID [1 byte]
	data[12] = defaultCollationID
	if cname := mc.cfg.Collation; cname != "" {
		colID, ok := collations[cname]
		if ok {
			data[12] = colID
		} else if len(mc.cfg.charsets) > 0 {
			// When cfg.charset is set, the collation is set by `SET NAMES <charset> COLLATE <collation>`.
			return fmt.Errorf("unknown collation: %q", cname)
		}
	}

	// Filler [23 bytes] (all 0x00)
	pos := 13
	for ; pos < 13+23; pos++ {
		data[pos] = 0
	}

	// SSL Connection Request Packet
	// http://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::SSLRequest
	if mc.cfg.TLS != nil {
		// Send TLS / SSL request packet
		if err := mc.writePacket(data[:(4+4+1+23)+4]); err != nil {
			return err
		}

		// Switch to TLS
		tlsConn := tls.Client(mc.netConn, mc.cfg.TLS)
		if err := tlsConn.Handshake(); err != nil {
			if cerr := mc.canceled.Value(); cerr != nil {
				return cerr
			}
			return err
		}
		mc.netConn = tlsConn
	}

	// User [null terminated string]
	if len(mc.cfg.User) > 0 {
		pos += copy(data[pos:], mc.cfg.User)
	}
	data[pos] = 0x00
	pos++

	// Auth Data [length encoded integer]
	pos += copy(data[pos:], authRespLEI)
	pos += copy(data[pos:], authResp)

	// Databasename [null terminated string]
	if len(mc.cfg.DBName) > 0 {
		pos += copy(data[pos:], mc.cfg.DBName)
		data[pos] = 0x00
		pos++
	}

	pos += copy(data[pos:], plugin)
	data[pos] = 0x00
	pos++

	// Connection Attributes
	if sendConnectAttrs {
		pos += copy(data[pos:], connAttrsLEI)
		pos += copy(data[pos:], []byte(mc.connector.encodedAttributes))
	}

	// Send Auth packet
	return mc.writePacket(data[:pos])
}

// http://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::AuthSwitchResponse
func (mc *mysqlConn) writeAuthSwitchPacket(authData []byte) error {
	pktLen := 4 + len(authData)
	data, err := mc.buf.takeBuffer(pktLen)
	if err != nil {
		mc.cleanup()
		return err
	}

	// Add the auth data [EOF]
	copy(data[4:], authData)
	return mc.writePacket(data)
}

/******************************************************************************
*                             Command Packets                                 *
******************************************************************************/

func (mc *mysqlConn) writeCommandPacket(command byte) error {
	// Reset Packet Sequence
	mc.resetSequence()

	data, err := mc.buf.takeSmallBuffer(4 + 1)
	if err != nil {
		return err
	}

	// Add command byte
	data[4] = command

	// Send CMD packet
	err = mc.writePacket(data)
	mc.syncSequence()
	return err
}

func (mc *mysqlConn) writeCommandPacketStr(command byte, arg string) error {
	// Reset Packet Sequence
	mc.resetSequence()

	pktLen := 1 + len(arg)
	data, err := mc.buf.takeBuffer(pktLen + 4)
	if err != nil {
		return err
	}

	// Add command byte
	data[4] = command

	// Add arg
	copy(data[5:], arg)

	// Send CMD packet
	err = mc.writePacket(data)
	mc.syncSequence()
	return err
}

func (mc *mysqlConn) writeCommandPacketUint32(command byte, arg uint32) error {
	// Reset Packet Sequence
	mc.resetSequence()

	data, err := mc.buf.takeSmallBuffer(4 + 1 + 4)
	if err != nil {
		return err
	}

	// Add command byte
	data[4] = command

	// Add arg [32 bit]
	binary.LittleEndian.PutUint32(data[5:], arg)

	// Send CMD packet
	err = mc.writePacket(data)
	mc.syncSequence()
	return err
}

/******************************************************************************
*                              Result Packets                                 *
******************************************************************************/

func (mc *mysqlConn) readAuthResult() ([]byte, string, error) {
	data, err := mc.readPacket()
	if err != nil {
		return nil, "", err
	}

	// packet indicator
	switch data[0] {

	case iOK:
		// resultUnchanged, since auth happens before any queries or
		// commands have been executed.
		return nil, "", mc.resultUnchanged().handleOkPacket(data)

	case iAuthMoreData:
		return data[1:], "", err

	case iEOF:
		if len(data) == 1 {
			// https://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::OldAuthSwitchRequest
			return nil, "mysql_old_password", nil
		}
		pluginEndIndex := bytes.IndexByte(data, 0x00)
		if pluginEndIndex < 0 {
			return nil, "", ErrMalformPkt
		}
		plugin := string(data[1:pluginEndIndex])
		authData := data[pluginEndIndex+1:]
		if len(authData) > 0 && authData[len(authData)-1] == 0 {
			authData = authData[:len(authData)-1]
		}
		return authData, plugin, nil

	default: // Error otherwise
		return nil, "", mc.handleErrorPacket(data)
	}
}

// Returns error if Packet is not a 'Result OK'-Packet
func (mc *okHandler) readResultOK() error {
	data, err := mc.conn().readPacket()
	if err != nil {
		return err
	}

	if data[0] == iOK {
		return mc.handleOkPacket(data)
	}
	return mc.conn().handleErrorPacket(data)
}

// Result Set Header Packet
// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_com_query_response.html
func (mc *okHandler) readResultSetHeaderPacket() (int, error) {
	// handleOkPacket replaces both values; other cases leave the values unchanged.
	mc.result.affectedRows = append(mc.result.affectedRows, 0)
	mc.result.insertIds = append(mc.result.insertIds, 0)

	data, err := mc.conn().readPacket()
	if err != nil {
		return 0, err
	}

	switch data[0] {
	case iOK:
		return 0, mc.handleOkPacket(data)

	case iERR:
		return 0, mc.conn().handleErrorPacket(data)

	case iLocalInFile:
		return 0, mc.handleInFileRequest(string(data[1:]))
	}

	// column count
	// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_com_query_response_text_resultset.html
	num, _, _ := readLengthEncodedInteger(data)
	// ignore remaining data in the packet. see #1478.
	return int(num), nil
}

// Error Packet
// http://dev.mysql.com/doc/internals/en/generic-response-packets.html#packet-ERR_Packet
func (mc *mysqlConn) handleErrorPacket(data []byte) error {
	if data[0] != iERR {
		return ErrMalformPkt
	}

	// 0xff [1 byte]

	// Error Number [16 bit uint]
	errno := binary.LittleEndian.Uint16(data[1:3])

	// 1792: ER_CANT_EXECUTE_IN_READ_ONLY_TRANSACTION
	// 1290: ER_OPTION_PREVENTS_STATEMENT (returned by Aurora during failover)
	// 1836: ER_READ_ONLY_MODE
	if (errno == 1792 || errno == 1290 || errno == 1836) && mc.cfg.RejectReadOnly {
		// Oops; we are connected to a read-only connection, and won't be able
		// to issue any write statements. Since RejectReadOnly is configured,
		// we throw away this connection hoping this one would have write
		// permission. This is specifically for a possible race condition
		// during failover (e.g. on AWS Aurora). See README.md for more.
		//
		// We explicitly close the connection before returning
		// driver.ErrBadConn to ensure that `database/sql` purges this
		// connection and initiates a new one for next statement next time.
		mc.Close()
		return driver.ErrBadConn
	}

	me := &MySQLError{Number: errno}

	pos := 3

	// SQL State [optional: # + 5bytes string]
	if data[3] == 0x23 {
		copy(me.SQLState[:], data[4:4+5])
		pos = 9
	}

	// Error Message [string]
	me.Message = string(data[pos:])

	return me
}

func readStatus(b []byte) statusFlag {
	return statusFlag(b[0]) | statusFlag(b[1])<<8
}

// Returns an instance of okHandler for codepaths where mysqlConn.result doesn't
// need to be cleared first (e.g. during authentication, or while additional
// resultsets are being fetched.)
func (mc *mysqlConn) resultUnchanged() *okHandler {
	return (*okHandler)(mc)
}

// okHandler represents the state of the connection when mysqlConn.result has
// been prepared for processing of OK packets.
//
// To correctly populate mysqlConn.result (updated by handleOkPacket()), all
// callpaths must either:
//
// 1. first clear it using clearResult(), or
// 2. confirm that they don't need to (by calling resultUnchanged()).
//
// Both return an instance of type *okHandler.
type okHandler mysqlConn

// Exposes the underlying type's methods.
func (mc *okHandler) conn() *mysqlConn {
	return (*mysqlConn)(mc)
}

// clearResult clears the connection's stored affectedRows and insertIds
// fields.
//
// It returns a handler that can process OK responses.
func (mc *mysqlConn) clearResult() *okHandler {
	mc.result = mysqlResult{}
	return (*okHandler)(mc)
}

// Ok Packet
// http://dev.mysql.com/doc/internals/en/generic-response-packets.html#packet-OK_Packet
func (mc *okHandler) handleOkPacket(data []byte) error {
	var n, m int
	var affectedRows, insertId uint64

	// 0x00 [1 byte]

	// Affected rows [Length Coded Binary]
	affectedRows, _, n = readLengthEncodedInteger(data[1:])

	// Insert id [Length Coded Binary]
	insertId, _, m = readLengthEncodedInteger(data[1+n:])

	// Update for the current statement result (only used by
	// readResultSetHeaderPacket).
	if len(mc.result.affectedRows) > 0 {
		mc.result.affectedRows[len(mc.result.affectedRows)-1] = int64(affectedRows)
	}
	if len(mc.result.insertIds) > 0 {
		mc.result.insertIds[len(mc.result.insertIds)-1] = int64(insertId)
	}

	// server_status [2 bytes]
	mc.status = readStatus(data[1+n+m : 1+n+m+2])
	if mc.status&statusMoreResultsExists != 0 {
		return nil
	}

	// warning count [2 bytes]

	return nil
}

// Read Packets as Field Packets until EOF-Packet or an Error appears
// http://dev.mysql.com/doc/internals/en/com-query-response.html#packet-Protocol::ColumnDefinition41
func (mc *mysqlConn) readColumns(count int) ([]mysqlField, error) {
	columns := make([]mysqlField, count)

	for i := 0; ; i++ {
		data, err := mc.readPacket()
		if err != nil {
			return nil, err
		}

		// EOF Packet
		if data[0] == iEOF && (len(data) == 5 || len(data) == 1) {
			if i == count {
				return columns, nil
			}
			return nil, fmt.Errorf("column count mismatch n:%d len:%d", count, len(columns))
		}

		// Catalog
		pos, err := skipLengthEncodedString(data)
		if err != nil {
			return nil, err
		}

		// Database [len coded string]
		n, err := skipLengthEncodedString(data[pos:])
		if err != nil {
			return nil, err
		}
		pos += n

		// Table [len coded string]
		if mc.cfg.ColumnsWithAlias {
			tableName, _, n, err := readLengthEncodedString(data[pos:])
			if err != nil {
				return nil, err
			}
			pos += n
			columns[i].tableName = string(tableName)
		} else {
			n, err = skipLengthEncodedString(data[pos:])
			if err != nil {
				return nil, err
			}
			pos += n
		}

		// Original table [len coded string]
		n, err = skipLengthEncodedString(data[pos:])
		if err != nil {
			return nil, err
		}
		pos += n

		// Name [len coded string]
		name, _, n, err := readLengthEncodedString(data[pos:])
		if err != nil {
			return nil, err
		}
		columns[i].name = string(name)
		pos += n

		// Original name [len coded string]
		n, err = skipLengthEncodedString(data[pos:])
		if err != nil {
			return nil, err
		}
		pos += n

		// Filler [uint8]
		pos++

		// Charset [charset, collation uint8]
		columns[i].charSet = data[pos]
		pos += 2

		// Length [uint32]
		columns[i].length = binary.LittleEndian.Uint32(data[pos : pos+4])
		pos += 4

		// Field type [uint8]
		columns[i].fieldType = fieldType(data[pos])
		pos++

		// Flags [uint16]
		columns[i].flags = fieldFlag(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2

		// Decimals [uint8]
		columns[i].decimals = data[pos]
		//pos++

		// Default value [len coded binary]
		//if pos < len(data) {
		//	defaultVal, _, err = bytesToLengthCodedBinary(data[pos:])
		//}
	}
}

// Read Packets as Field Packets until EOF-Packet or an Error appears
// http://dev.mysql.com/doc/internals/en/com-query-response.html#packet-ProtocolText::ResultsetRow
func (rows *textRows) readRow(dest []driver.Value) error {
	mc := rows.mc

	if rows.rs.done {
		return io.EOF
	}

	data, err := mc.readPacket()
	if err != nil {
		return err
	}

	// EOF Packet
	if data[0] == iEOF && len(data) == 5 {
		// server_status [2 bytes]
		rows.mc.status = readStatus(data[3:])
		rows.rs.done = true
		if !rows.HasNextResultSet() {
			rows.mc = nil
		}
		return io.EOF
	}
	if data[0] == iERR {
		rows.mc = nil
		return mc.handleErrorPacket(data)
	}

	// RowSet Packet
	var (
		n      int
		isNull bool
		pos    int = 0
	)

	for i := range dest {
		// Read bytes and convert to string
		var buf []byte
		buf, isNull, n, err = readLengthEncodedString(data[pos:])
		pos += n

		if err != nil {
			return err
		}

		if isNull {
			dest[i] = nil
			continue
		}

		switch rows.rs.columns[i].fieldType {
		case fieldTypeTimestamp,
			fieldTypeDateTime,
			fieldTypeDate,
			fieldTypeNewDate:
			if mc.parseTime {
				dest[i], err = parseDateTime(buf, mc.cfg.Loc)
			} else {
				dest[i] = buf
			}

		case fieldTypeTiny, fieldTypeShort, fieldTypeInt24, fieldTypeYear, fieldTypeLong:
			dest[i], err = strconv.ParseInt(string(buf), 10, 64)

		case fieldTypeLongLong:
			if rows.rs.columns[i].flags&flagUnsigned != 0 {
				dest[i], err = strconv.ParseUint(string(buf), 10, 64)
			} else {
				dest[i], err = strconv.ParseInt(string(buf), 10, 64)
			}

		case fieldTypeFloat:
			var d float64
			d, err = strconv.ParseFloat(string(buf), 32)
			dest[i] = float32(d)

		case fieldTypeDouble:
			dest[i], err = strconv.ParseFloat(string(buf), 64)

		default:
			dest[i] = buf
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// Reads Packets until EOF-Packet or an Error appears. Returns count of Packets read
func (mc *mysqlConn) readUntilEOF() error {
	for {
		data, err := mc.readPacket()
		if err != nil {
			return err
		}

		switch data[0] {
		case iERR:
			return mc.handleErrorPacket(data)
		case iEOF:
			if len(data) == 5 {
				mc.status = readStatus(data[3:])
			}
			return nil
		}
	}
}

/******************************************************************************
*                           Prepared Statements                               *
******************************************************************************/

// Prepare Result Packets
// http://dev.mysql.com/doc/internals/en/com-stmt-prepare-response.html
func (stmt *mysqlStmt) readPrepareResultPacket() (uint16, error) {
	data, err := stmt.mc.readPacket()
	if err == nil {
		// packet indicator [1 byte]
		if data[0] != iOK {
			return 0, stmt.mc.handleErrorPacket(data)
		}

		// statement id [4 bytes]
		stmt.id = binary.LittleEndian.Uint32(data[1:5])

		// Column count [16 bit uint]
		columnCount := binary.LittleEndian.Uint16(data[5:7])

		// Param count [16 bit uint]
		stmt.paramCount = int(binary.LittleEndian.Uint16(data[7:9]))

		// Reserved [8 bit]

		// Warning count [16 bit uint]

		return columnCount, nil
	}
	return 0, err
}

// http://dev.mysql.com/doc/internals/en/com-stmt-send-long-data.html
func (stmt *mysqlStmt) writeCommandLongData(paramID int, arg []byte) error {
	maxLen := stmt.mc.maxAllowedPacket - 1
	pktLen := maxLen

	// After the header (bytes 0-3) follows before the data:
	// 1 byte command
	// 4 bytes stmtID
	// 2 bytes paramID
	const dataOffset = 1 + 4 + 2

	// Cannot use the write buffer since
	// a) the buffer is too small
	// b) it is in use
	data := make([]byte, 4+1+4+2+len(arg))

	copy(data[4+dataOffset:], arg)

	for argLen := len(arg); argLen > 0; argLen -= pktLen - dataOffset {
		if dataOffset+argLen < maxLen {
			pktLen = dataOffset + argLen
		}

		// Add command byte [1 byte]
		data[4] = comStmtSendLongData

		// Add stmtID [32 bit]
		binary.LittleEndian.PutUint32(data[5:], stmt.id)

		// Add paramID [16 bit]
		binary.LittleEndian.PutUint16(data[9:], uint16(paramID))

		// Send CMD packet
		err := stmt.mc.writePacket(data[:4+pktLen])
		// Every COM_LONG_DATA packet reset Packet Sequence
		stmt.mc.resetSequence()
		if err == nil {
			data = data[pktLen-dataOffset:]
			continue
		}
		return err
	}

	return nil
}

// Execute Prepared Statement
// http://dev.mysql.com/doc/internals/en/com-stmt-execute.html
func (stmt *mysqlStmt) writeExecutePacket(args []driver.Value) error {
	if len(args) != stmt.paramCount {
		return fmt.Errorf(
			"argument count mismatch (got: %d; has: %d)",
			len(args),
			stmt.paramCount,
		)
	}

	const minPktLen = 4 + 1 + 4 + 1 + 4
	mc := stmt.mc

	// Determine threshold dynamically to avoid packet size shortage.
	longDataSize := mc.maxAllowedPacket / (stmt.paramCount + 1)
	if longDataSize < 64 {
		longDataSize = 64
	}

	// Reset packet-sequence
	mc.resetSequence()

	var data []byte
	var err error

	if len(args) == 0 {
		data, err = mc.buf.takeBuffer(minPktLen)
	} else {
		data, err = mc.buf.takeCompleteBuffer()
		// In this case the len(data) == cap(data) which is used to optimise the flow below.
	}
	if err != nil {
		return err
	}

	// command [1 byte]
	data[4] = comStmtExecute

	// statement_id [4 bytes]
	binary.LittleEndian.PutUint32(data[5:], stmt.id)

	// flags (0: CURSOR_TYPE_NO_CURSOR) [1 byte]
	data[9] = 0x00

	// iteration_count (uint32(1)) [4 bytes]
	binary.LittleEndian.PutUint32(data[10:], 1)

	if len(args) > 0 {
		pos := minPktLen

		var nullMask []byte
		if maskLen, typesLen := (len(args)+7)/8, 1+2*len(args); pos+maskLen+typesLen >= cap(data) {
			// buffer has to be extended but we don't know by how much so
			// we depend on append after all data with known sizes fit.
			// We stop at that because we deal with a lot of columns here
			// which makes the required allocation size hard to guess.
			tmp := make([]byte, pos+maskLen+typesLen)
			copy(tmp[:pos], data[:pos])
			data = tmp
			nullMask = data[pos : pos+maskLen]
			// No need to clean nullMask as make ensures that.
			pos += maskLen
		} else {
			nullMask = data[pos : pos+maskLen]
			for i := range nullMask {
				nullMask[i] = 0
			}
			pos += maskLen
		}

		// newParameterBoundFlag 1 [1 byte]
		data[pos] = 0x01
		pos++

		// type of each parameter [len(args)*2 bytes]
		paramTypes := data[pos:]
		pos += len(args) * 2

		// value of each parameter [n bytes]
		paramValues := data[pos:pos]
		valuesCap := cap(paramValues)

		for i, arg := range args {
			// build NULL-bitmap
			if arg == nil {
				nullMask[i/8] |= 1 << (uint(i) & 7)
				paramTypes[i+i] = byte(fieldTypeNULL)
				paramTypes[i+i+1] = 0x00
				continue
			}

			if v, ok := arg.(json.RawMessage); ok {
				arg = []byte(v)
			}
			// cache types and values
			switch v := arg.(type) {
			case int64:
				paramTypes[i+i] = byte(fieldTypeLongLong)
				paramTypes[i+i+1] = 0x00
				paramValues = binary.LittleEndian.AppendUint64(paramValues, uint64(v))

			case uint64:
				paramTypes[i+i] = byte(fieldTypeLongLong)
				paramTypes[i+i+1] = 0x80 // type is unsigned
				paramValues = binary.LittleEndian.AppendUint64(paramValues, uint64(v))

			case float64:
				paramTypes[i+i] = byte(fieldTypeDouble)
				paramTypes[i+i+1] = 0x00
				paramValues = binary.LittleEndian.AppendUint64(paramValues, math.Float64bits(v))

			case bool:
				paramTypes[i+i] = byte(fieldTypeTiny)
				paramTypes[i+i+1] = 0x00

				if v {
					paramValues = append(paramValues, 0x01)
				} else {
					paramValues = append(paramValues, 0x00)
				}

			case []byte:
				// Common case (non-nil value) first
				if v != nil {
					paramTypes[i+i] = byte(fieldTypeString)
					paramTypes[i+i+1] = 0x00

					if len(v) < longDataSize {
						paramValues = appendLengthEncodedInteger(paramValues,
							uint64(len(v)),
						)
						paramValues = append(paramValues, v...)
					} else {
						if err := stmt.writeCommandLongData(i, v); err != nil {
							return err
						}
					}
					continue
				}

				// Handle []byte(nil) as a NULL value
				nullMask[i/8] |= 1 << (uint(i) & 7)
				paramTypes[i+i] = byte(fieldTypeNULL)
				paramTypes[i+i+1] = 0x00

			case string:
				paramTypes[i+i] = byte(fieldTypeString)
				paramTypes[i+i+1] = 0x00

				if len(v) < longDataSize {
					paramValues = appendLengthEncodedInteger(paramValues,
						uint64(len(v)),
					)
					paramValues = append(paramValues, v...)
				} else {
					if err := stmt.writeCommandLongData(i, []byte(v)); err != nil {
						return err
					}
				}

			case time.Time:
				paramTypes[i+i] = byte(fieldTypeString)
				paramTypes[i+i+1] = 0x00

				var a [64]byte
				var b = a[:0]

				if v.IsZero() {
					b = append(b, "0000-00-00"...)
				} else {
					b, err = appendDateTime(b, v.In(mc.cfg.Loc), mc.cfg.timeTruncate)
					if err != nil {
						return err
					}
				}

				paramValues = appendLengthEncodedInteger(paramValues,
					uint64(len(b)),
				)
				paramValues = append(paramValues, b...)

			default:
				return fmt.Errorf("cannot convert type: %T", arg)
			}
		}

		// Check if param values exceeded the available buffer
		// In that case we must build the data packet with the new values buffer
		if valuesCap != cap(paramValues) {
			data = append(data[:pos], paramValues...)
			mc.buf.store(data) // allow this buffer to be reused
		}

		pos += len(paramValues)
		data = data[:pos]
	}

	err = mc.writePacket(data)
	mc.syncSequence()
	return err
}

// For each remaining resultset in the stream, discards its rows and updates
// mc.affectedRows and mc.insertIds.
func (mc *okHandler) discardResults() error {
	for mc.status&statusMoreResultsExists != 0 {
		resLen, err := mc.readResultSetHeaderPacket()
		if err != nil {
			return err
		}
		if resLen > 0 {
			// columns
			if err := mc.conn().readUntilEOF(); err != nil {
				return err
			}
			// rows
			if err := mc.conn().readUntilEOF(); err != nil {
				return err
			}
		}
	}
	return nil
}

// http://dev.mysql.com/doc/internals/en/binary-protocol-resultset-row.html
func (rows *binaryRows) readRow(dest []driver.Value) error {
	data, err := rows.mc.readPacket()
	if err != nil {
		return err
	}

	// packet indicator [1 byte]
	if data[0] != iOK {
		// EOF Packet
		if data[0] == iEOF && len(data) == 5 {
			rows.mc.status = readStatus(data[3:])
			rows.rs.done = true
			if !rows.HasNextResultSet() {
				rows.mc = nil
			}
			return io.EOF
		}
		mc := rows.mc
		rows.mc = nil

		// Error otherwise
		return mc.handleErrorPacket(data)
	}

	// NULL-bitmap,  [(column-count + 7 + 2) / 8 bytes]
	pos := 1 + (len(dest)+7+2)>>3
	nullMask := data[1:pos]

	for i := range dest {
		// Field is NULL
		// (byte >> bit-pos) % 2 == 1
		if ((nullMask[(i+2)>>3] >> uint((i+2)&7)) & 1) == 1 {
			dest[i] = nil
			continue
		}

		// Convert to byte-coded string
		switch rows.rs.columns[i].fieldType {
		case fieldTypeNULL:
			dest[i] = nil
			continue

		// Numeric Types
		case fieldTypeTiny:
			if rows.rs.columns[i].flags&flagUnsigned != 0 {
				dest[i] = int64(data[pos])
			} else {
				dest[i] = int64(int8(data[pos]))
			}
			pos++
			continue

		case fieldTypeShort, fieldTypeYear:
			if rows.rs.columns[i].flags&flagUnsigned != 0 {
				dest[i] = int64(binary.LittleEndian.Uint16(data[pos : pos+2]))
			} else {
				dest[i] = int64(int16(binary.LittleEndian.Uint16(data[pos : pos+2])))
			}
			pos += 2
			continue

		case fieldTypeInt24, fieldTypeLong:
			if rows.rs.columns[i].flags&flagUnsigned != 0 {
				dest[i] = int64(binary.LittleEndian.Uint32(data[pos : pos+4]))
			} else {
				dest[i] = int64(int32(binary.LittleEndian.Uint32(data[pos : pos+4])))
			}
			pos += 4
			continue

		case fieldTypeLongLong:
			if rows.rs.columns[i].flags&flagUnsigned != 0 {
				val := binary.LittleEndian.Uint64(data[pos : pos+8])
				if val > math.MaxInt64 {
					dest[i] = uint64ToString(val)
				} else {
					dest[i] = int64(val)
				}
			} else {
				dest[i] = int64(binary.LittleEndian.Uint64(data[pos : pos+8]))
			}
			pos += 8
			continue

		case fieldTypeFloat:
			dest[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[pos : pos+4]))
			pos += 4
			continue

		case fieldTypeDouble:
			dest[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[pos : pos+8]))
			pos += 8
			continue

		// Length coded Binary Strings
		case fieldTypeDecimal, fieldTypeNewDecimal, fieldTypeVarChar,
			fieldTypeBit, fieldTypeEnum, fieldTypeSet, fieldTypeTinyBLOB,
			fieldTypeMediumBLOB, fieldTypeLongBLOB, fieldTypeBLOB,
			fieldTypeVarString, fieldTypeString, fieldTypeGeometry, fieldTypeJSON,
			fieldTypeVector:
			var isNull bool
			var n int
			dest[i], isNull, n, err = readLengthEncodedString(data[pos:])
			pos += n
			if err == nil {
				if !isNull {
					continue
				} else {
					dest[i] = nil
					continue
				}
			}
			return err

		case
			fieldTypeDate, fieldTypeNewDate, // Date YYYY-MM-DD
			fieldTypeTime,                         // Time [-][H]HH:MM:SS[.fractal]
			fieldTypeTimestamp, fieldTypeDateTime: // Timestamp YYYY-MM-DD HH:MM:SS[.fractal]

			num, isNull, n := readLengthEncodedInteger(data[pos:])
			pos += n

			switch {
			case isNull:
				dest[i] = nil
				continue
			case rows.rs.columns[i].fieldType == fieldTypeTime:
				// database/sql does not support an equivalent to TIME, return a string
				var dstlen uint8
				switch decimals := rows.rs.columns[i].decimals; decimals {
				case 0x00, 0x1f:
					dstlen = 8
				case 1, 2, 3, 4, 5, 6:
					dstlen = 8 + 1 + decimals
				default:
					return fmt.Errorf(
						"protocol error, illegal decimals value %d",
						rows.rs.columns[i].decimals,
					)
				}
				dest[i], err = formatBinaryTime(data[pos:pos+int(num)], dstlen)
			case rows.mc.parseTime:
				dest[i], err = parseBinaryDateTime(num, data[pos:], rows.mc.cfg.Loc)
			default:
				var dstlen uint8
				if rows.rs.columns[i].fieldType == fieldTypeDate {
					dstlen = 10
				} else {
					switch decimals := rows.rs.columns[i].decimals; decimals {
					case 0x00, 0x1f:
						dstlen = 19
					case 1, 2, 3, 4, 5, 6:
						dstlen = 19 + 1 + decimals
					default:
						return fmt.Errorf(
							"protocol error, illegal decimals value %d",
							rows.rs.columns[i].decimals,
						)
					}
				}
				dest[i], err = formatBinaryDateTime(data[pos:pos+int(num)], dstlen)
			}

			if err == nil {
				pos += int(num)
				continue
			} else {
				return err
			}

		// Please report if this happens!
		default:
			return fmt.Errorf("unknown field type %d", rows.rs.columns[i].fieldType)
		}
	}

	return nil
}
