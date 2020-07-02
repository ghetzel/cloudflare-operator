package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=A;AAAA;CAA;CERT;CNAME;DNSKEY;DS;LOC;MX;NAPTR;NS;PTR;SMIMEA;SPF;SRV;SSHFP;TLSA;TXT;URI
type RecordType string

const (
	A      RecordType = `A`
	AAAA              = `AAAA`
	CAA               = `CAA`
	CERT              = `CERT`
	CNAME             = `CNAME`
	DNSKEY            = `DNSKEY`
	DS                = `DS`
	LOC               = `LOC`
	MX                = `MX`
	NAPTR             = `NAPTR`
	NS                = `NS`
	PTR               = `PTR`
	SMIMEA            = `SMIMEA`
	SPF               = `SPF`
	SRV               = `SRV`
	SSHFP             = `SSHFP`
	TLSA              = `TLSA`
	TXT               = `TXT`
	URI               = `URI`
)

// CloudflareRecordSpec defines the desired state of CloudflareRecord
type CloudflareRecordSpec struct {
	Zone string `json:"zone"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Default=A
	Type RecordType `json:"type"`
	Name string     `json:"name"`

	// +kubebuilder:validation:Optional
	Content string `json:"content"`

	// +kubebuilder:validation:Optional
	Service string `json:"service"`

	// +kubebuilder:validation:Optional
	Ingress string `json:"ingress"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Default=1
	TTL int `json:"ttl"`

	Proxied bool `json:"proxied"`

	// +kubebuilder:validation:Optional
	Priority int `json:"priority"`
}

// CloudflareRecordStatus defines the observed state of CloudflareRecord
type CloudflareRecordStatus struct {
	Zone     string     `json:"zone"`
	Type     RecordType `json:"type"`
	Name     string     `json:"name"`
	Content  string     `json:"content"`
	Service  string     `json:"service"`
	TTL      int        `json:"ttl"`
	Priority int        `json:"priority"`
	Proxied  bool       `json:"proxied"`
	ZoneID   string     `json:"zone_id"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CloudflareRecord is the Schema for the cloudflarerecords API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=cloudflarerecords,scope=Namespaced
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.type`
// +kubebuilder:printcolumn:name="Proxied",type=boolean,JSONPath=`.status.proxied`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.status.content`
type CloudflareRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareRecordSpec   `json:"spec,omitempty"`
	Status CloudflareRecordStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CloudflareRecordList contains a list of CloudflareRecord
type CloudflareRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareRecord{}, &CloudflareRecordList{})
}
