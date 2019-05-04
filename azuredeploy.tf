variable "location" {
  description   = "Azure datacenter to deploy to."
  default       = "westus2"
}

variable "signalr_name_prefix" {
  description = "Input your unique Azure Service Bus Namespace name"
  default     = "azuresrtests"
}

variable "resource_group_name_prefix" {
  description = "Resource group to provision test infrastructure in."
  default     = "signalr-go-tests"
}

# Data resources used to get SubID and Tennant Info
data "azurerm_client_config" "current" {}

resource "random_string" "name" {
  length  = 8
  upper   = false
  special = false
  number  = false
}

# Create resource group for all of the things
resource "azurerm_resource_group" "test" {
  name      = "${var.resource_group_name_prefix}-${random_string.name.result}"
  location  = "${var.location}"
}

resource "azurerm_signalr_service" "test" {
  name                = "${var.resource_group_name_prefix}-${random_string.name.result}"
  location            = "${azurerm_resource_group.test.location}"
  resource_group_name = "${azurerm_resource_group.test.name}"

  sku {
    name     = "Free_F1"
    capacity = 1
  }
}

output "SIGNALR_CONNECTION_STRING" {
  value     = "${azurerm_signalr_service.test.primary_connection_string}"
  sensitive = true
}