package ruleimport

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net/netip"

	"github.com/klauspost/compress/zstd"
)

// Read the data-only MRSv1 envelope used by Mihomo. Bound decompression and every
// length before examining the trie/range body; no Core process is started here.
func parseMRS(raw []byte, o Options) (Provider, error) {
	decoder, err := zstd.NewReader(bytes.NewReader(raw), zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return Provider{}, errors.New("MRS 压缩数据无效")
	}
	defer decoder.Close()
	data, err := io.ReadAll(io.LimitReader(decoder, (32<<20)+1))
	if err != nil || len(data) > 32<<20 || len(data) < 22 || !bytes.Equal(data[:4], []byte{'M', 'R', 'S', 1}) {
		return Provider{}, errors.New("MRS 文件无效、版本不支持或解压后超过 32 MiB")
	}
	behavior := ""
	switch data[4] {
	case 0:
		behavior = "domain"
	case 1:
		behavior = "ipcidr"
	default:
		return Provider{}, errors.New("MRS 仅支持 domain 和 ipcidr")
	}
	if o.Behavior != "" && o.Behavior != "auto" && o.Behavior != behavior {
		return Provider{}, errors.New("选择的规则类型与 MRS 内容不匹配")
	}
	count := int64(binary.BigEndian.Uint64(data[5:13]))
	extra := int64(binary.BigEndian.Uint64(data[13:21]))
	if count < 1 || count > MaxItems || extra < 0 || extra > int64(len(data)-22) {
		return Provider{}, errors.New("MRS 条目数或保留字段无效")
	}
	r := bytes.NewReader(data[21+extra:])
	version, err := r.ReadByte()
	if err != nil || version != 1 {
		return Provider{}, errors.New("MRS 数据结构版本不支持")
	}
	read := func(unit int) ([]byte, error) {
		var n int64
		if binary.Read(r, binary.BigEndian, &n) != nil || n < 1 || n > int64(r.Len()/unit) {
			return nil, errors.New("MRS 数据截断或长度无效")
		}
		v := make([]byte, int(n)*unit)
		_, err := io.ReadFull(r, v)
		return v, err
	}
	if behavior == "domain" {
		leaves, e := read(8)
		if e != nil {
			return Provider{}, e
		}
		bitmap, e := read(8)
		if e != nil {
			return Provider{}, e
		}
		labels, e := read(1)
		if e != nil {
			return Provider{}, e
		}
		// LOUDS has one terminator per node and one zero per label/edge. Check the
		// reachable tree, bitmap capacity and terminal nodes, not just the magic.
		leafBit := func(i int) bool { return binary.BigEndian.Uint64(leaves[(i/64)*8:])&(uint64(1)<<uint(i%64)) != 0 }
		bit := func(i int) bool { return binary.BigEndian.Uint64(bitmap[(i/64)*8:])&(uint64(1)<<uint(i%64)) != 0 }
		pos, edges, nodes, terminals := 0, 0, 1, 0
		for node := 0; node < nodes; node++ {
			if node >= len(leaves)*8 {
				return Provider{}, errors.New("MRS 域名树叶节点容量不足")
			}
			if leafBit(node) {
				terminals++
			}
			for {
				if pos >= len(bitmap)*8 {
					return Provider{}, errors.New("MRS 域名树位图截断")
				}
				v := bit(pos)
				pos++
				if v {
					break
				}
				edges++
				nodes++
				if edges > len(labels) {
					return Provider{}, errors.New("MRS 域名树标签截断")
				}
			}
		}
		if edges != len(labels) || terminals < 1 || leafBit(0) {
			return Provider{}, errors.New("MRS 域名树结构无效")
		}
		// Remaining bitmap/leaf bits must be padding.
		for i := pos; i < len(bitmap)*8; i++ {
			if bit(i) {
				return Provider{}, errors.New("MRS 域名树存在尾部数据")
			}
		}
		for i := nodes; i < len(leaves)*8; i++ {
			if leafBit(i) {
				return Provider{}, errors.New("MRS 域名树存在无效叶节点")
			}
		}
		for _, label := range labels {
			if label == 0 {
				return Provider{}, errors.New("MRS 域名标签无效")
			}
		}
		if terminals < int(count) || terminals > int(count)*2 {
			return Provider{}, errors.New("MRS 域名计数不匹配")
		}
	} else {
		ranges, e := read(32)
		if e != nil {
			return Provider{}, e
		}
		var previous netip.Addr
		for i := 0; i < len(ranges); i += 32 {
			var a, b [16]byte
			copy(a[:], ranges[i:i+16])
			copy(b[:], ranges[i+16:i+32])
			from, to := netip.AddrFrom16(a).Unmap(), netip.AddrFrom16(b).Unmap()
			if from.BitLen() != to.BitLen() || from.Compare(to) > 0 || previous.IsValid() && previous.Compare(from) >= 0 {
				return Provider{}, errors.New("MRS IP 范围无效")
			}
			previous = to
		}
	}
	if r.Len() != 0 {
		return Provider{}, errors.New("MRS 存在未识别的尾部内容")
	}
	p := Provider{Name: Name(o.Name), Behavior: behavior, Format: "mrs", Count: int(count), Sample: []string{}, Asset: raw, Config: map[string]any{"type": "file", "behavior": behavior, "format": "mrs"}}
	if o.URL != "" {
		p.Asset = nil
		p.Config["type"] = "http"
		p.Config["url"] = o.URL
		p.Config["interval"] = 86400
		p.Config["path"] = "./rule-import/" + assetID([]byte(o.URL)) + ".mrs"
	}
	return p, nil
}
