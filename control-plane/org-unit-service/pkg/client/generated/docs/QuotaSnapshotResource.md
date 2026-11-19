# QuotaSnapshotResource

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Deserved** | **NullableFloat64** |  | 
**Limit** | **NullableFloat64** |  | 
**OverQuotaWeight** | **NullableFloat64** |  | 

## Methods

### NewQuotaSnapshotResource

`func NewQuotaSnapshotResource(deserved NullableFloat64, limit NullableFloat64, overQuotaWeight NullableFloat64, ) *QuotaSnapshotResource`

NewQuotaSnapshotResource instantiates a new QuotaSnapshotResource object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaSnapshotResourceWithDefaults

`func NewQuotaSnapshotResourceWithDefaults() *QuotaSnapshotResource`

NewQuotaSnapshotResourceWithDefaults instantiates a new QuotaSnapshotResource object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetDeserved

`func (o *QuotaSnapshotResource) GetDeserved() float64`

GetDeserved returns the Deserved field if non-nil, zero value otherwise.

### GetDeservedOk

`func (o *QuotaSnapshotResource) GetDeservedOk() (*float64, bool)`

GetDeservedOk returns a tuple with the Deserved field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDeserved

`func (o *QuotaSnapshotResource) SetDeserved(v float64)`

SetDeserved sets Deserved field to given value.


### SetDeservedNil

`func (o *QuotaSnapshotResource) SetDeservedNil(b bool)`

 SetDeservedNil sets the value for Deserved to be an explicit nil

### UnsetDeserved
`func (o *QuotaSnapshotResource) UnsetDeserved()`

UnsetDeserved ensures that no value is present for Deserved, not even an explicit nil
### GetLimit

`func (o *QuotaSnapshotResource) GetLimit() float64`

GetLimit returns the Limit field if non-nil, zero value otherwise.

### GetLimitOk

`func (o *QuotaSnapshotResource) GetLimitOk() (*float64, bool)`

GetLimitOk returns a tuple with the Limit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLimit

`func (o *QuotaSnapshotResource) SetLimit(v float64)`

SetLimit sets Limit field to given value.


### SetLimitNil

`func (o *QuotaSnapshotResource) SetLimitNil(b bool)`

 SetLimitNil sets the value for Limit to be an explicit nil

### UnsetLimit
`func (o *QuotaSnapshotResource) UnsetLimit()`

UnsetLimit ensures that no value is present for Limit, not even an explicit nil
### GetOverQuotaWeight

`func (o *QuotaSnapshotResource) GetOverQuotaWeight() float64`

GetOverQuotaWeight returns the OverQuotaWeight field if non-nil, zero value otherwise.

### GetOverQuotaWeightOk

`func (o *QuotaSnapshotResource) GetOverQuotaWeightOk() (*float64, bool)`

GetOverQuotaWeightOk returns a tuple with the OverQuotaWeight field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOverQuotaWeight

`func (o *QuotaSnapshotResource) SetOverQuotaWeight(v float64)`

SetOverQuotaWeight sets OverQuotaWeight field to given value.


### SetOverQuotaWeightNil

`func (o *QuotaSnapshotResource) SetOverQuotaWeightNil(b bool)`

 SetOverQuotaWeightNil sets the value for OverQuotaWeight to be an explicit nil

### UnsetOverQuotaWeight
`func (o *QuotaSnapshotResource) UnsetOverQuotaWeight()`

UnsetOverQuotaWeight ensures that no value is present for OverQuotaWeight, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


