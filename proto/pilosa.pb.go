// Code generated by protoc-gen-go. DO NOT EDIT.
// source: pilosa.proto

package pilosa

import (
	context "context"
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type QueryPQLRequest struct {
	Index                string   `protobuf:"bytes,1,opt,name=index,proto3" json:"index,omitempty"`
	Pql                  string   `protobuf:"bytes,2,opt,name=pql,proto3" json:"pql,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *QueryPQLRequest) Reset()         { *m = QueryPQLRequest{} }
func (m *QueryPQLRequest) String() string { return proto.CompactTextString(m) }
func (*QueryPQLRequest) ProtoMessage()    {}
func (*QueryPQLRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{0}
}

func (m *QueryPQLRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_QueryPQLRequest.Unmarshal(m, b)
}
func (m *QueryPQLRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_QueryPQLRequest.Marshal(b, m, deterministic)
}
func (m *QueryPQLRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_QueryPQLRequest.Merge(m, src)
}
func (m *QueryPQLRequest) XXX_Size() int {
	return xxx_messageInfo_QueryPQLRequest.Size(m)
}
func (m *QueryPQLRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_QueryPQLRequest.DiscardUnknown(m)
}

var xxx_messageInfo_QueryPQLRequest proto.InternalMessageInfo

func (m *QueryPQLRequest) GetIndex() string {
	if m != nil {
		return m.Index
	}
	return ""
}

func (m *QueryPQLRequest) GetPql() string {
	if m != nil {
		return m.Pql
	}
	return ""
}

type QuerySQLRequest struct {
	Sql                  string   `protobuf:"bytes,1,opt,name=sql,proto3" json:"sql,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *QuerySQLRequest) Reset()         { *m = QuerySQLRequest{} }
func (m *QuerySQLRequest) String() string { return proto.CompactTextString(m) }
func (*QuerySQLRequest) ProtoMessage()    {}
func (*QuerySQLRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{1}
}

func (m *QuerySQLRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_QuerySQLRequest.Unmarshal(m, b)
}
func (m *QuerySQLRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_QuerySQLRequest.Marshal(b, m, deterministic)
}
func (m *QuerySQLRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_QuerySQLRequest.Merge(m, src)
}
func (m *QuerySQLRequest) XXX_Size() int {
	return xxx_messageInfo_QuerySQLRequest.Size(m)
}
func (m *QuerySQLRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_QuerySQLRequest.DiscardUnknown(m)
}

var xxx_messageInfo_QuerySQLRequest proto.InternalMessageInfo

func (m *QuerySQLRequest) GetSql() string {
	if m != nil {
		return m.Sql
	}
	return ""
}

type StatusError struct {
	Code                 uint32   `protobuf:"varint,1,opt,name=Code,proto3" json:"Code,omitempty"`
	Message              string   `protobuf:"bytes,2,opt,name=Message,proto3" json:"Message,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StatusError) Reset()         { *m = StatusError{} }
func (m *StatusError) String() string { return proto.CompactTextString(m) }
func (*StatusError) ProtoMessage()    {}
func (*StatusError) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{2}
}

func (m *StatusError) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StatusError.Unmarshal(m, b)
}
func (m *StatusError) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StatusError.Marshal(b, m, deterministic)
}
func (m *StatusError) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StatusError.Merge(m, src)
}
func (m *StatusError) XXX_Size() int {
	return xxx_messageInfo_StatusError.Size(m)
}
func (m *StatusError) XXX_DiscardUnknown() {
	xxx_messageInfo_StatusError.DiscardUnknown(m)
}

var xxx_messageInfo_StatusError proto.InternalMessageInfo

func (m *StatusError) GetCode() uint32 {
	if m != nil {
		return m.Code
	}
	return 0
}

func (m *StatusError) GetMessage() string {
	if m != nil {
		return m.Message
	}
	return ""
}

type RowResponse struct {
	Headers              []*ColumnInfo     `protobuf:"bytes,1,rep,name=headers,proto3" json:"headers,omitempty"`
	Columns              []*ColumnResponse `protobuf:"bytes,2,rep,name=columns,proto3" json:"columns,omitempty"`
	StatusError          *StatusError      `protobuf:"bytes,3,opt,name=StatusError,proto3" json:"StatusError,omitempty"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *RowResponse) Reset()         { *m = RowResponse{} }
func (m *RowResponse) String() string { return proto.CompactTextString(m) }
func (*RowResponse) ProtoMessage()    {}
func (*RowResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{3}
}

func (m *RowResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RowResponse.Unmarshal(m, b)
}
func (m *RowResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RowResponse.Marshal(b, m, deterministic)
}
func (m *RowResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RowResponse.Merge(m, src)
}
func (m *RowResponse) XXX_Size() int {
	return xxx_messageInfo_RowResponse.Size(m)
}
func (m *RowResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_RowResponse.DiscardUnknown(m)
}

var xxx_messageInfo_RowResponse proto.InternalMessageInfo

func (m *RowResponse) GetHeaders() []*ColumnInfo {
	if m != nil {
		return m.Headers
	}
	return nil
}

func (m *RowResponse) GetColumns() []*ColumnResponse {
	if m != nil {
		return m.Columns
	}
	return nil
}

func (m *RowResponse) GetStatusError() *StatusError {
	if m != nil {
		return m.StatusError
	}
	return nil
}

type Row struct {
	Columns              []*ColumnResponse `protobuf:"bytes,1,rep,name=columns,proto3" json:"columns,omitempty"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *Row) Reset()         { *m = Row{} }
func (m *Row) String() string { return proto.CompactTextString(m) }
func (*Row) ProtoMessage()    {}
func (*Row) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{4}
}

func (m *Row) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Row.Unmarshal(m, b)
}
func (m *Row) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Row.Marshal(b, m, deterministic)
}
func (m *Row) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Row.Merge(m, src)
}
func (m *Row) XXX_Size() int {
	return xxx_messageInfo_Row.Size(m)
}
func (m *Row) XXX_DiscardUnknown() {
	xxx_messageInfo_Row.DiscardUnknown(m)
}

var xxx_messageInfo_Row proto.InternalMessageInfo

func (m *Row) GetColumns() []*ColumnResponse {
	if m != nil {
		return m.Columns
	}
	return nil
}

type TableResponse struct {
	Headers              []*ColumnInfo `protobuf:"bytes,1,rep,name=headers,proto3" json:"headers,omitempty"`
	Rows                 []*Row        `protobuf:"bytes,2,rep,name=rows,proto3" json:"rows,omitempty"`
	StatusError          *StatusError  `protobuf:"bytes,3,opt,name=StatusError,proto3" json:"StatusError,omitempty"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *TableResponse) Reset()         { *m = TableResponse{} }
func (m *TableResponse) String() string { return proto.CompactTextString(m) }
func (*TableResponse) ProtoMessage()    {}
func (*TableResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{5}
}

func (m *TableResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TableResponse.Unmarshal(m, b)
}
func (m *TableResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TableResponse.Marshal(b, m, deterministic)
}
func (m *TableResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TableResponse.Merge(m, src)
}
func (m *TableResponse) XXX_Size() int {
	return xxx_messageInfo_TableResponse.Size(m)
}
func (m *TableResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_TableResponse.DiscardUnknown(m)
}

var xxx_messageInfo_TableResponse proto.InternalMessageInfo

func (m *TableResponse) GetHeaders() []*ColumnInfo {
	if m != nil {
		return m.Headers
	}
	return nil
}

func (m *TableResponse) GetRows() []*Row {
	if m != nil {
		return m.Rows
	}
	return nil
}

func (m *TableResponse) GetStatusError() *StatusError {
	if m != nil {
		return m.StatusError
	}
	return nil
}

type ColumnInfo struct {
	Name                 string   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Datatype             string   `protobuf:"bytes,2,opt,name=datatype,proto3" json:"datatype,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ColumnInfo) Reset()         { *m = ColumnInfo{} }
func (m *ColumnInfo) String() string { return proto.CompactTextString(m) }
func (*ColumnInfo) ProtoMessage()    {}
func (*ColumnInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{6}
}

func (m *ColumnInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ColumnInfo.Unmarshal(m, b)
}
func (m *ColumnInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ColumnInfo.Marshal(b, m, deterministic)
}
func (m *ColumnInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ColumnInfo.Merge(m, src)
}
func (m *ColumnInfo) XXX_Size() int {
	return xxx_messageInfo_ColumnInfo.Size(m)
}
func (m *ColumnInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_ColumnInfo.DiscardUnknown(m)
}

var xxx_messageInfo_ColumnInfo proto.InternalMessageInfo

func (m *ColumnInfo) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *ColumnInfo) GetDatatype() string {
	if m != nil {
		return m.Datatype
	}
	return ""
}

type ColumnResponse struct {
	// Types that are valid to be assigned to ColumnVal:
	//	*ColumnResponse_StringVal
	//	*ColumnResponse_Uint64Val
	//	*ColumnResponse_Int64Val
	//	*ColumnResponse_BoolVal
	//	*ColumnResponse_BlobVal
	//	*ColumnResponse_Uint64ArrayVal
	//	*ColumnResponse_StringArrayVal
	//	*ColumnResponse_Float64Val
	//	*ColumnResponse_DecimalVal
	ColumnVal            isColumnResponse_ColumnVal `protobuf_oneof:"columnVal"`
	XXX_NoUnkeyedLiteral struct{}                   `json:"-"`
	XXX_unrecognized     []byte                     `json:"-"`
	XXX_sizecache        int32                      `json:"-"`
}

func (m *ColumnResponse) Reset()         { *m = ColumnResponse{} }
func (m *ColumnResponse) String() string { return proto.CompactTextString(m) }
func (*ColumnResponse) ProtoMessage()    {}
func (*ColumnResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{7}
}

func (m *ColumnResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ColumnResponse.Unmarshal(m, b)
}
func (m *ColumnResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ColumnResponse.Marshal(b, m, deterministic)
}
func (m *ColumnResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ColumnResponse.Merge(m, src)
}
func (m *ColumnResponse) XXX_Size() int {
	return xxx_messageInfo_ColumnResponse.Size(m)
}
func (m *ColumnResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ColumnResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ColumnResponse proto.InternalMessageInfo

type isColumnResponse_ColumnVal interface {
	isColumnResponse_ColumnVal()
}

type ColumnResponse_StringVal struct {
	StringVal string `protobuf:"bytes,1,opt,name=stringVal,proto3,oneof"`
}

type ColumnResponse_Uint64Val struct {
	Uint64Val uint64 `protobuf:"varint,2,opt,name=uint64Val,proto3,oneof"`
}

type ColumnResponse_Int64Val struct {
	Int64Val int64 `protobuf:"varint,3,opt,name=int64Val,proto3,oneof"`
}

type ColumnResponse_BoolVal struct {
	BoolVal bool `protobuf:"varint,4,opt,name=boolVal,proto3,oneof"`
}

type ColumnResponse_BlobVal struct {
	BlobVal []byte `protobuf:"bytes,5,opt,name=blobVal,proto3,oneof"`
}

type ColumnResponse_Uint64ArrayVal struct {
	Uint64ArrayVal *Uint64Array `protobuf:"bytes,6,opt,name=uint64ArrayVal,proto3,oneof"`
}

type ColumnResponse_StringArrayVal struct {
	StringArrayVal *StringArray `protobuf:"bytes,7,opt,name=stringArrayVal,proto3,oneof"`
}

type ColumnResponse_Float64Val struct {
	Float64Val float64 `protobuf:"fixed64,8,opt,name=float64Val,proto3,oneof"`
}

type ColumnResponse_DecimalVal struct {
	DecimalVal *Decimal `protobuf:"bytes,9,opt,name=decimalVal,proto3,oneof"`
}

func (*ColumnResponse_StringVal) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_Uint64Val) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_Int64Val) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_BoolVal) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_BlobVal) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_Uint64ArrayVal) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_StringArrayVal) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_Float64Val) isColumnResponse_ColumnVal() {}

func (*ColumnResponse_DecimalVal) isColumnResponse_ColumnVal() {}

func (m *ColumnResponse) GetColumnVal() isColumnResponse_ColumnVal {
	if m != nil {
		return m.ColumnVal
	}
	return nil
}

func (m *ColumnResponse) GetStringVal() string {
	if x, ok := m.GetColumnVal().(*ColumnResponse_StringVal); ok {
		return x.StringVal
	}
	return ""
}

func (m *ColumnResponse) GetUint64Val() uint64 {
	if x, ok := m.GetColumnVal().(*ColumnResponse_Uint64Val); ok {
		return x.Uint64Val
	}
	return 0
}

func (m *ColumnResponse) GetInt64Val() int64 {
	if x, ok := m.GetColumnVal().(*ColumnResponse_Int64Val); ok {
		return x.Int64Val
	}
	return 0
}

func (m *ColumnResponse) GetBoolVal() bool {
	if x, ok := m.GetColumnVal().(*ColumnResponse_BoolVal); ok {
		return x.BoolVal
	}
	return false
}

func (m *ColumnResponse) GetBlobVal() []byte {
	if x, ok := m.GetColumnVal().(*ColumnResponse_BlobVal); ok {
		return x.BlobVal
	}
	return nil
}

func (m *ColumnResponse) GetUint64ArrayVal() *Uint64Array {
	if x, ok := m.GetColumnVal().(*ColumnResponse_Uint64ArrayVal); ok {
		return x.Uint64ArrayVal
	}
	return nil
}

func (m *ColumnResponse) GetStringArrayVal() *StringArray {
	if x, ok := m.GetColumnVal().(*ColumnResponse_StringArrayVal); ok {
		return x.StringArrayVal
	}
	return nil
}

func (m *ColumnResponse) GetFloat64Val() float64 {
	if x, ok := m.GetColumnVal().(*ColumnResponse_Float64Val); ok {
		return x.Float64Val
	}
	return 0
}

func (m *ColumnResponse) GetDecimalVal() *Decimal {
	if x, ok := m.GetColumnVal().(*ColumnResponse_DecimalVal); ok {
		return x.DecimalVal
	}
	return nil
}

// XXX_OneofWrappers is for the internal use of the proto package.
func (*ColumnResponse) XXX_OneofWrappers() []interface{} {
	return []interface{}{
		(*ColumnResponse_StringVal)(nil),
		(*ColumnResponse_Uint64Val)(nil),
		(*ColumnResponse_Int64Val)(nil),
		(*ColumnResponse_BoolVal)(nil),
		(*ColumnResponse_BlobVal)(nil),
		(*ColumnResponse_Uint64ArrayVal)(nil),
		(*ColumnResponse_StringArrayVal)(nil),
		(*ColumnResponse_Float64Val)(nil),
		(*ColumnResponse_DecimalVal)(nil),
	}
}

type Decimal struct {
	Value                int64    `protobuf:"varint,1,opt,name=value,proto3" json:"value,omitempty"`
	Scale                int64    `protobuf:"varint,2,opt,name=scale,proto3" json:"scale,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Decimal) Reset()         { *m = Decimal{} }
func (m *Decimal) String() string { return proto.CompactTextString(m) }
func (*Decimal) ProtoMessage()    {}
func (*Decimal) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{8}
}

func (m *Decimal) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Decimal.Unmarshal(m, b)
}
func (m *Decimal) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Decimal.Marshal(b, m, deterministic)
}
func (m *Decimal) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Decimal.Merge(m, src)
}
func (m *Decimal) XXX_Size() int {
	return xxx_messageInfo_Decimal.Size(m)
}
func (m *Decimal) XXX_DiscardUnknown() {
	xxx_messageInfo_Decimal.DiscardUnknown(m)
}

var xxx_messageInfo_Decimal proto.InternalMessageInfo

func (m *Decimal) GetValue() int64 {
	if m != nil {
		return m.Value
	}
	return 0
}

func (m *Decimal) GetScale() int64 {
	if m != nil {
		return m.Scale
	}
	return 0
}

type InspectRequest struct {
	Index                string     `protobuf:"bytes,1,opt,name=index,proto3" json:"index,omitempty"`
	Columns              *IdsOrKeys `protobuf:"bytes,2,opt,name=columns,proto3" json:"columns,omitempty"`
	FilterFields         []string   `protobuf:"bytes,3,rep,name=filterFields,proto3" json:"filterFields,omitempty"`
	Limit                uint64     `protobuf:"varint,4,opt,name=limit,proto3" json:"limit,omitempty"`
	Offset               uint64     `protobuf:"varint,5,opt,name=offset,proto3" json:"offset,omitempty"`
	Query                string     `protobuf:"bytes,6,opt,name=query,proto3" json:"query,omitempty"`
	XXX_NoUnkeyedLiteral struct{}   `json:"-"`
	XXX_unrecognized     []byte     `json:"-"`
	XXX_sizecache        int32      `json:"-"`
}

func (m *InspectRequest) Reset()         { *m = InspectRequest{} }
func (m *InspectRequest) String() string { return proto.CompactTextString(m) }
func (*InspectRequest) ProtoMessage()    {}
func (*InspectRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{9}
}

func (m *InspectRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_InspectRequest.Unmarshal(m, b)
}
func (m *InspectRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_InspectRequest.Marshal(b, m, deterministic)
}
func (m *InspectRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_InspectRequest.Merge(m, src)
}
func (m *InspectRequest) XXX_Size() int {
	return xxx_messageInfo_InspectRequest.Size(m)
}
func (m *InspectRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_InspectRequest.DiscardUnknown(m)
}

var xxx_messageInfo_InspectRequest proto.InternalMessageInfo

func (m *InspectRequest) GetIndex() string {
	if m != nil {
		return m.Index
	}
	return ""
}

func (m *InspectRequest) GetColumns() *IdsOrKeys {
	if m != nil {
		return m.Columns
	}
	return nil
}

func (m *InspectRequest) GetFilterFields() []string {
	if m != nil {
		return m.FilterFields
	}
	return nil
}

func (m *InspectRequest) GetLimit() uint64 {
	if m != nil {
		return m.Limit
	}
	return 0
}

func (m *InspectRequest) GetOffset() uint64 {
	if m != nil {
		return m.Offset
	}
	return 0
}

func (m *InspectRequest) GetQuery() string {
	if m != nil {
		return m.Query
	}
	return ""
}

type Uint64Array struct {
	Vals                 []uint64 `protobuf:"varint,1,rep,packed,name=vals,proto3" json:"vals,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Uint64Array) Reset()         { *m = Uint64Array{} }
func (m *Uint64Array) String() string { return proto.CompactTextString(m) }
func (*Uint64Array) ProtoMessage()    {}
func (*Uint64Array) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{10}
}

func (m *Uint64Array) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Uint64Array.Unmarshal(m, b)
}
func (m *Uint64Array) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Uint64Array.Marshal(b, m, deterministic)
}
func (m *Uint64Array) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Uint64Array.Merge(m, src)
}
func (m *Uint64Array) XXX_Size() int {
	return xxx_messageInfo_Uint64Array.Size(m)
}
func (m *Uint64Array) XXX_DiscardUnknown() {
	xxx_messageInfo_Uint64Array.DiscardUnknown(m)
}

var xxx_messageInfo_Uint64Array proto.InternalMessageInfo

func (m *Uint64Array) GetVals() []uint64 {
	if m != nil {
		return m.Vals
	}
	return nil
}

type StringArray struct {
	Vals                 []string `protobuf:"bytes,1,rep,name=vals,proto3" json:"vals,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StringArray) Reset()         { *m = StringArray{} }
func (m *StringArray) String() string { return proto.CompactTextString(m) }
func (*StringArray) ProtoMessage()    {}
func (*StringArray) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{11}
}

func (m *StringArray) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StringArray.Unmarshal(m, b)
}
func (m *StringArray) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StringArray.Marshal(b, m, deterministic)
}
func (m *StringArray) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StringArray.Merge(m, src)
}
func (m *StringArray) XXX_Size() int {
	return xxx_messageInfo_StringArray.Size(m)
}
func (m *StringArray) XXX_DiscardUnknown() {
	xxx_messageInfo_StringArray.DiscardUnknown(m)
}

var xxx_messageInfo_StringArray proto.InternalMessageInfo

func (m *StringArray) GetVals() []string {
	if m != nil {
		return m.Vals
	}
	return nil
}

type IdsOrKeys struct {
	// Types that are valid to be assigned to Type:
	//	*IdsOrKeys_Ids
	//	*IdsOrKeys_Keys
	Type                 isIdsOrKeys_Type `protobuf_oneof:"type"`
	XXX_NoUnkeyedLiteral struct{}         `json:"-"`
	XXX_unrecognized     []byte           `json:"-"`
	XXX_sizecache        int32            `json:"-"`
}

func (m *IdsOrKeys) Reset()         { *m = IdsOrKeys{} }
func (m *IdsOrKeys) String() string { return proto.CompactTextString(m) }
func (*IdsOrKeys) ProtoMessage()    {}
func (*IdsOrKeys) Descriptor() ([]byte, []int) {
	return fileDescriptor_ef0691a44d1e275c, []int{12}
}

func (m *IdsOrKeys) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_IdsOrKeys.Unmarshal(m, b)
}
func (m *IdsOrKeys) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_IdsOrKeys.Marshal(b, m, deterministic)
}
func (m *IdsOrKeys) XXX_Merge(src proto.Message) {
	xxx_messageInfo_IdsOrKeys.Merge(m, src)
}
func (m *IdsOrKeys) XXX_Size() int {
	return xxx_messageInfo_IdsOrKeys.Size(m)
}
func (m *IdsOrKeys) XXX_DiscardUnknown() {
	xxx_messageInfo_IdsOrKeys.DiscardUnknown(m)
}

var xxx_messageInfo_IdsOrKeys proto.InternalMessageInfo

type isIdsOrKeys_Type interface {
	isIdsOrKeys_Type()
}

type IdsOrKeys_Ids struct {
	Ids *Uint64Array `protobuf:"bytes,1,opt,name=ids,proto3,oneof"`
}

type IdsOrKeys_Keys struct {
	Keys *StringArray `protobuf:"bytes,2,opt,name=keys,proto3,oneof"`
}

func (*IdsOrKeys_Ids) isIdsOrKeys_Type() {}

func (*IdsOrKeys_Keys) isIdsOrKeys_Type() {}

func (m *IdsOrKeys) GetType() isIdsOrKeys_Type {
	if m != nil {
		return m.Type
	}
	return nil
}

func (m *IdsOrKeys) GetIds() *Uint64Array {
	if x, ok := m.GetType().(*IdsOrKeys_Ids); ok {
		return x.Ids
	}
	return nil
}

func (m *IdsOrKeys) GetKeys() *StringArray {
	if x, ok := m.GetType().(*IdsOrKeys_Keys); ok {
		return x.Keys
	}
	return nil
}

// XXX_OneofWrappers is for the internal use of the proto package.
func (*IdsOrKeys) XXX_OneofWrappers() []interface{} {
	return []interface{}{
		(*IdsOrKeys_Ids)(nil),
		(*IdsOrKeys_Keys)(nil),
	}
}

func init() {
	proto.RegisterType((*QueryPQLRequest)(nil), "pilosa.QueryPQLRequest")
	proto.RegisterType((*QuerySQLRequest)(nil), "pilosa.QuerySQLRequest")
	proto.RegisterType((*StatusError)(nil), "pilosa.StatusError")
	proto.RegisterType((*RowResponse)(nil), "pilosa.RowResponse")
	proto.RegisterType((*Row)(nil), "pilosa.Row")
	proto.RegisterType((*TableResponse)(nil), "pilosa.TableResponse")
	proto.RegisterType((*ColumnInfo)(nil), "pilosa.ColumnInfo")
	proto.RegisterType((*ColumnResponse)(nil), "pilosa.ColumnResponse")
	proto.RegisterType((*Decimal)(nil), "pilosa.Decimal")
	proto.RegisterType((*InspectRequest)(nil), "pilosa.InspectRequest")
	proto.RegisterType((*Uint64Array)(nil), "pilosa.Uint64Array")
	proto.RegisterType((*StringArray)(nil), "pilosa.StringArray")
	proto.RegisterType((*IdsOrKeys)(nil), "pilosa.IdsOrKeys")
}

func init() { proto.RegisterFile("pilosa.proto", fileDescriptor_ef0691a44d1e275c) }

var fileDescriptor_ef0691a44d1e275c = []byte{
	// 718 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x55, 0xdd, 0x52, 0xd3, 0x4e,
	0x14, 0x6f, 0x48, 0x68, 0x9b, 0x53, 0xbe, 0xfe, 0xcb, 0x5f, 0xcc, 0x30, 0x8e, 0xc6, 0x78, 0x61,
	0x1c, 0x1d, 0x06, 0x51, 0x74, 0x54, 0xbc, 0x00, 0xd4, 0x29, 0xa3, 0x8e, 0x65, 0x11, 0xee, 0xb7,
	0xcd, 0x16, 0x33, 0x6e, 0xb3, 0x6d, 0x36, 0x05, 0xfb, 0x02, 0xbe, 0x81, 0x6f, 0xe0, 0x5b, 0x78,
	0xef, 0x73, 0x39, 0xbb, 0x9b, 0x4d, 0x13, 0xb4, 0x0e, 0x72, 0xb7, 0xe7, 0xfc, 0x7e, 0xe7, 0x2b,
	0xe7, 0x23, 0xb0, 0x30, 0x8c, 0x19, 0x17, 0x64, 0x63, 0x98, 0xf2, 0x8c, 0xa3, 0xba, 0x96, 0x82,
	0x67, 0xb0, 0x7c, 0x38, 0xa6, 0xe9, 0xa4, 0x73, 0xf8, 0x0e, 0xd3, 0xd1, 0x98, 0x8a, 0x0c, 0xfd,
	0x0f, 0xf3, 0x71, 0x12, 0xd1, 0x2f, 0x9e, 0xe5, 0x5b, 0xa1, 0x8b, 0xb5, 0x80, 0x56, 0xc0, 0x1e,
	0x8e, 0x98, 0x37, 0xa7, 0x74, 0xf2, 0x19, 0xdc, 0xc9, 0x4d, 0x8f, 0xa6, 0xa6, 0x2b, 0x60, 0x8b,
	0x11, 0xcb, 0x0d, 0xe5, 0x33, 0x78, 0x01, 0xad, 0xa3, 0x8c, 0x64, 0x63, 0xf1, 0x3a, 0x4d, 0x79,
	0x8a, 0x10, 0x38, 0xfb, 0x3c, 0xa2, 0x8a, 0xb1, 0x88, 0xd5, 0x1b, 0x79, 0xd0, 0x78, 0x4f, 0x85,
	0x20, 0xa7, 0x34, 0xf7, 0x6e, 0xc4, 0xe0, 0xbb, 0x05, 0x2d, 0xcc, 0xcf, 0x31, 0x15, 0x43, 0x9e,
	0x08, 0x8a, 0x1e, 0x40, 0xe3, 0x13, 0x25, 0x11, 0x4d, 0x85, 0x67, 0xf9, 0x76, 0xd8, 0xda, 0x42,
	0x1b, 0x79, 0x51, 0xfb, 0x9c, 0x8d, 0x07, 0xc9, 0x41, 0xd2, 0xe7, 0xd8, 0x50, 0xd0, 0x26, 0x34,
	0x7a, 0x4a, 0x2d, 0xbc, 0x39, 0xc5, 0x5e, 0xab, 0xb2, 0x8d, 0x5b, 0x6c, 0x68, 0x68, 0xbb, 0x92,
	0xac, 0x67, 0xfb, 0x56, 0xd8, 0xda, 0x5a, 0x35, 0x56, 0x25, 0x08, 0x97, 0x79, 0xc1, 0x53, 0xb0,
	0x31, 0x3f, 0x2f, 0xc7, 0xb3, 0x2e, 0x15, 0x2f, 0xf8, 0x66, 0xc1, 0xe2, 0x47, 0xd2, 0x65, 0xf4,
	0x8a, 0x15, 0xde, 0x02, 0x27, 0xe5, 0xe7, 0xa6, 0xbc, 0x96, 0xa1, 0xca, 0x4f, 0xa6, 0x80, 0xab,
	0x16, 0xb4, 0x03, 0x30, 0x0d, 0x27, 0x7b, 0x96, 0x90, 0x01, 0xcd, 0xbb, 0xaa, 0xde, 0x68, 0x1d,
	0x9a, 0x11, 0xc9, 0x48, 0x36, 0x19, 0x9a, 0xa6, 0x15, 0x72, 0xf0, 0xd5, 0x86, 0xa5, 0x6a, 0xc5,
	0xe8, 0x26, 0xb8, 0x22, 0x4b, 0xe3, 0xe4, 0xf4, 0x84, 0xe4, 0xd3, 0xd1, 0xae, 0xe1, 0xa9, 0x4a,
	0xe2, 0xe3, 0x38, 0xc9, 0x9e, 0x3c, 0x96, 0xb8, 0xf4, 0xe7, 0x48, 0xbc, 0x50, 0xa1, 0x1b, 0xd0,
	0x2c, 0x60, 0x59, 0x84, 0xdd, 0xae, 0xe1, 0x42, 0x83, 0xd6, 0xa1, 0xd1, 0xe5, 0x9c, 0x49, 0xd0,
	0xf1, 0xad, 0xb0, 0xd9, 0xae, 0x61, 0xa3, 0x50, 0x18, 0xe3, 0x5d, 0x89, 0xcd, 0xfb, 0x56, 0xb8,
	0xa0, 0x30, 0xad, 0x40, 0x2f, 0x61, 0x49, 0x87, 0xd8, 0x4d, 0x53, 0x32, 0x91, 0x94, 0x7a, 0xf5,
	0x03, 0x1d, 0x4f, 0xd1, 0x76, 0x0d, 0x5f, 0x20, 0x4b, 0x73, 0x5d, 0x41, 0x61, 0xde, 0xb8, 0xf8,
	0x7d, 0x0b, 0x54, 0x9a, 0x57, 0xc9, 0xc8, 0x07, 0xe8, 0x33, 0x4e, 0xf2, 0xaa, 0x9a, 0xbe, 0x15,
	0x5a, 0xed, 0x1a, 0x2e, 0xe9, 0xd0, 0x43, 0x80, 0x88, 0xf6, 0xe2, 0x01, 0x51, 0xa5, 0xb9, 0xca,
	0xf9, 0xb2, 0x71, 0xfe, 0x4a, 0x23, 0xd2, 0x64, 0x4a, 0xda, 0x6b, 0x81, 0xab, 0x87, 0xeb, 0x84,
	0xb0, 0x60, 0x1b, 0x1a, 0x39, 0x4b, 0xee, 0xf4, 0x19, 0x61, 0x63, 0xdd, 0x44, 0x1b, 0x6b, 0x41,
	0x6a, 0x45, 0x8f, 0x30, 0xdd, 0x42, 0x1b, 0x6b, 0x21, 0xf8, 0x61, 0xc1, 0xd2, 0x41, 0x22, 0x86,
	0xb4, 0x97, 0xfd, 0xfd, 0x24, 0xdc, 0x2f, 0x2f, 0x98, 0x4c, 0xee, 0x3f, 0x93, 0xdc, 0x41, 0x24,
	0x3e, 0xa4, 0x6f, 0xe9, 0x44, 0x4c, 0x77, 0x2b, 0x80, 0x85, 0x7e, 0xcc, 0x32, 0x9a, 0xbe, 0x89,
	0x29, 0x8b, 0x84, 0x67, 0xfb, 0x76, 0xe8, 0xe2, 0x8a, 0x4e, 0x86, 0x61, 0xf1, 0x20, 0xce, 0x54,
	0x1b, 0x1d, 0xac, 0x05, 0xb4, 0x06, 0x75, 0xde, 0xef, 0x0b, 0x9a, 0xa9, 0x0e, 0x3a, 0x38, 0x97,
	0x24, 0x7b, 0x24, 0xef, 0x8f, 0xea, 0x9a, 0x8b, 0xb5, 0x10, 0xdc, 0x86, 0x56, 0xa9, 0x6d, 0x72,
	0x78, 0xcf, 0x08, 0xd3, 0xdb, 0xe4, 0x60, 0xf5, 0x96, 0x94, 0x52, 0x6b, 0x2a, 0x14, 0x37, 0xa7,
	0x9c, 0x82, 0x5b, 0xd4, 0x80, 0xee, 0x82, 0x1d, 0x47, 0x42, 0xd5, 0x3e, 0x73, 0x38, 0x24, 0x03,
	0xdd, 0x03, 0xe7, 0x33, 0x9d, 0x98, 0xaf, 0x31, 0x63, 0x0e, 0x14, 0x65, 0xaf, 0x0e, 0x8e, 0x5c,
	0x96, 0xad, 0x9f, 0x73, 0x50, 0xef, 0x28, 0x1a, 0xda, 0x81, 0xa6, 0xb9, 0xa7, 0xe8, 0xba, 0xb1,
	0xbd, 0x70, 0x61, 0xd7, 0x57, 0xcb, 0x4b, 0x9e, 0xaf, 0x57, 0x50, 0xdb, 0xb4, 0xd0, 0x2e, 0x2c,
	0x1a, 0xee, 0x71, 0x42, 0xd2, 0xc9, 0x6c, 0x17, 0xd7, 0x0c, 0x50, 0x39, 0x3d, 0x41, 0xad, 0x48,
	0xa0, 0xf3, 0x5b, 0x02, 0x9d, 0x7f, 0x48, 0xa0, 0xf3, 0xe7, 0x04, 0x3a, 0x97, 0x48, 0xe0, 0x39,
	0x34, 0xf2, 0xc1, 0x43, 0xc5, 0xed, 0xac, 0x4e, 0xe2, 0xcc, 0xf0, 0xdd, 0xba, 0xfa, 0xaf, 0x3d,
	0xfa, 0x15, 0x00, 0x00, 0xff, 0xff, 0x7e, 0x8d, 0x53, 0x0b, 0xe7, 0x06, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// PilosaClient is the client API for Pilosa service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type PilosaClient interface {
	QuerySQL(ctx context.Context, in *QuerySQLRequest, opts ...grpc.CallOption) (Pilosa_QuerySQLClient, error)
	QuerySQLUnary(ctx context.Context, in *QuerySQLRequest, opts ...grpc.CallOption) (*TableResponse, error)
	QueryPQL(ctx context.Context, in *QueryPQLRequest, opts ...grpc.CallOption) (Pilosa_QueryPQLClient, error)
	QueryPQLUnary(ctx context.Context, in *QueryPQLRequest, opts ...grpc.CallOption) (*TableResponse, error)
	Inspect(ctx context.Context, in *InspectRequest, opts ...grpc.CallOption) (Pilosa_InspectClient, error)
}

type pilosaClient struct {
	cc *grpc.ClientConn
}

func NewPilosaClient(cc *grpc.ClientConn) PilosaClient {
	return &pilosaClient{cc}
}

func (c *pilosaClient) QuerySQL(ctx context.Context, in *QuerySQLRequest, opts ...grpc.CallOption) (Pilosa_QuerySQLClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Pilosa_serviceDesc.Streams[0], "/pilosa.Pilosa/QuerySQL", opts...)
	if err != nil {
		return nil, err
	}
	x := &pilosaQuerySQLClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Pilosa_QuerySQLClient interface {
	Recv() (*RowResponse, error)
	grpc.ClientStream
}

type pilosaQuerySQLClient struct {
	grpc.ClientStream
}

func (x *pilosaQuerySQLClient) Recv() (*RowResponse, error) {
	m := new(RowResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *pilosaClient) QuerySQLUnary(ctx context.Context, in *QuerySQLRequest, opts ...grpc.CallOption) (*TableResponse, error) {
	out := new(TableResponse)
	err := c.cc.Invoke(ctx, "/pilosa.Pilosa/QuerySQLUnary", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pilosaClient) QueryPQL(ctx context.Context, in *QueryPQLRequest, opts ...grpc.CallOption) (Pilosa_QueryPQLClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Pilosa_serviceDesc.Streams[1], "/pilosa.Pilosa/QueryPQL", opts...)
	if err != nil {
		return nil, err
	}
	x := &pilosaQueryPQLClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Pilosa_QueryPQLClient interface {
	Recv() (*RowResponse, error)
	grpc.ClientStream
}

type pilosaQueryPQLClient struct {
	grpc.ClientStream
}

func (x *pilosaQueryPQLClient) Recv() (*RowResponse, error) {
	m := new(RowResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *pilosaClient) QueryPQLUnary(ctx context.Context, in *QueryPQLRequest, opts ...grpc.CallOption) (*TableResponse, error) {
	out := new(TableResponse)
	err := c.cc.Invoke(ctx, "/pilosa.Pilosa/QueryPQLUnary", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pilosaClient) Inspect(ctx context.Context, in *InspectRequest, opts ...grpc.CallOption) (Pilosa_InspectClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Pilosa_serviceDesc.Streams[2], "/pilosa.Pilosa/Inspect", opts...)
	if err != nil {
		return nil, err
	}
	x := &pilosaInspectClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Pilosa_InspectClient interface {
	Recv() (*RowResponse, error)
	grpc.ClientStream
}

type pilosaInspectClient struct {
	grpc.ClientStream
}

func (x *pilosaInspectClient) Recv() (*RowResponse, error) {
	m := new(RowResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// PilosaServer is the server API for Pilosa service.
type PilosaServer interface {
	QuerySQL(*QuerySQLRequest, Pilosa_QuerySQLServer) error
	QuerySQLUnary(context.Context, *QuerySQLRequest) (*TableResponse, error)
	QueryPQL(*QueryPQLRequest, Pilosa_QueryPQLServer) error
	QueryPQLUnary(context.Context, *QueryPQLRequest) (*TableResponse, error)
	Inspect(*InspectRequest, Pilosa_InspectServer) error
}

// UnimplementedPilosaServer can be embedded to have forward compatible implementations.
type UnimplementedPilosaServer struct {
}

func (*UnimplementedPilosaServer) QuerySQL(req *QuerySQLRequest, srv Pilosa_QuerySQLServer) error {
	return status.Errorf(codes.Unimplemented, "method QuerySQL not implemented")
}
func (*UnimplementedPilosaServer) QuerySQLUnary(ctx context.Context, req *QuerySQLRequest) (*TableResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QuerySQLUnary not implemented")
}
func (*UnimplementedPilosaServer) QueryPQL(req *QueryPQLRequest, srv Pilosa_QueryPQLServer) error {
	return status.Errorf(codes.Unimplemented, "method QueryPQL not implemented")
}
func (*UnimplementedPilosaServer) QueryPQLUnary(ctx context.Context, req *QueryPQLRequest) (*TableResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryPQLUnary not implemented")
}
func (*UnimplementedPilosaServer) Inspect(req *InspectRequest, srv Pilosa_InspectServer) error {
	return status.Errorf(codes.Unimplemented, "method Inspect not implemented")
}

func RegisterPilosaServer(s *grpc.Server, srv PilosaServer) {
	s.RegisterService(&_Pilosa_serviceDesc, srv)
}

func _Pilosa_QuerySQL_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(QuerySQLRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(PilosaServer).QuerySQL(m, &pilosaQuerySQLServer{stream})
}

type Pilosa_QuerySQLServer interface {
	Send(*RowResponse) error
	grpc.ServerStream
}

type pilosaQuerySQLServer struct {
	grpc.ServerStream
}

func (x *pilosaQuerySQLServer) Send(m *RowResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _Pilosa_QuerySQLUnary_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QuerySQLRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PilosaServer).QuerySQLUnary(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pilosa.Pilosa/QuerySQLUnary",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PilosaServer).QuerySQLUnary(ctx, req.(*QuerySQLRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Pilosa_QueryPQL_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(QueryPQLRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(PilosaServer).QueryPQL(m, &pilosaQueryPQLServer{stream})
}

type Pilosa_QueryPQLServer interface {
	Send(*RowResponse) error
	grpc.ServerStream
}

type pilosaQueryPQLServer struct {
	grpc.ServerStream
}

func (x *pilosaQueryPQLServer) Send(m *RowResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _Pilosa_QueryPQLUnary_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryPQLRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PilosaServer).QueryPQLUnary(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pilosa.Pilosa/QueryPQLUnary",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PilosaServer).QueryPQLUnary(ctx, req.(*QueryPQLRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Pilosa_Inspect_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(InspectRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(PilosaServer).Inspect(m, &pilosaInspectServer{stream})
}

type Pilosa_InspectServer interface {
	Send(*RowResponse) error
	grpc.ServerStream
}

type pilosaInspectServer struct {
	grpc.ServerStream
}

func (x *pilosaInspectServer) Send(m *RowResponse) error {
	return x.ServerStream.SendMsg(m)
}

var _Pilosa_serviceDesc = grpc.ServiceDesc{
	ServiceName: "pilosa.Pilosa",
	HandlerType: (*PilosaServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "QuerySQLUnary",
			Handler:    _Pilosa_QuerySQLUnary_Handler,
		},
		{
			MethodName: "QueryPQLUnary",
			Handler:    _Pilosa_QueryPQLUnary_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "QuerySQL",
			Handler:       _Pilosa_QuerySQL_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "QueryPQL",
			Handler:       _Pilosa_QueryPQL_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "Inspect",
			Handler:       _Pilosa_Inspect_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "pilosa.proto",
}
