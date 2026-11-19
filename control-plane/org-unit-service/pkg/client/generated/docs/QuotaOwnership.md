# QuotaOwnership

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Epoch** | **string** |  | 
**KubernetesUid** | **string** |  | 
**Pools** | [**[]QuotaOwnershipPool**](QuotaOwnershipPool.md) |  | 

## Methods

### NewQuotaOwnership

`func NewQuotaOwnership(epoch string, kubernetesUid string, pools []QuotaOwnershipPool, ) *QuotaOwnership`

NewQuotaOwnership instantiates a new QuotaOwnership object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaOwnershipWithDefaults

`func NewQuotaOwnershipWithDefaults() *QuotaOwnership`

NewQuotaOwnershipWithDefaults instantiates a new QuotaOwnership object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetEpoch

`func (o *QuotaOwnership) GetEpoch() string`

GetEpoch returns the Epoch field if non-nil, zero value otherwise.

### GetEpochOk

`func (o *QuotaOwnership) GetEpochOk() (*string, bool)`

GetEpochOk returns a tuple with the Epoch field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEpoch

`func (o *QuotaOwnership) SetEpoch(v string)`

SetEpoch sets Epoch field to given value.


### GetKubernetesUid

`func (o *QuotaOwnership) GetKubernetesUid() string`

GetKubernetesUid returns the KubernetesUid field if non-nil, zero value otherwise.

### GetKubernetesUidOk

`func (o *QuotaOwnership) GetKubernetesUidOk() (*string, bool)`

GetKubernetesUidOk returns a tuple with the KubernetesUid field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKubernetesUid

`func (o *QuotaOwnership) SetKubernetesUid(v string)`

SetKubernetesUid sets KubernetesUid field to given value.


### GetPools

`func (o *QuotaOwnership) GetPools() []QuotaOwnershipPool`

GetPools returns the Pools field if non-nil, zero value otherwise.

### GetPoolsOk

`func (o *QuotaOwnership) GetPoolsOk() (*[]QuotaOwnershipPool, bool)`

GetPoolsOk returns a tuple with the Pools field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPools

`func (o *QuotaOwnership) SetPools(v []QuotaOwnershipPool)`

SetPools sets Pools field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


