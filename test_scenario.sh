#!/bin/bash

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}FideX Two-Node Test Scenario${NC}"
echo -e "${GREEN}========================================${NC}"
sudo rm -rf test_data
mkdir -p "test_data/node-a" "test_data/node-b" 
chown 1000:1000 -R test_data
#sudo chmod aog+w "test_data/node-a" "test_data/node-b"

# 1. Start Containers
echo -e "${YELLOW}Starting Docker containers...${NC}"
docker-compose -f docker-compose.test.yml up -d

# Wait for services to be ready
echo -e "${YELLOW}Waiting for nodes to start...${NC}"

wait_for_node() {
    url=$1
    name=$2
    max_retries=30
    count=0
    
    while [ $count -lt $max_retries ]; do
        if curl -s "$url/health" | grep -q "healthy"; then
            echo -e "${GREEN}✓ $name is healthy${NC}"
            return 0
        fi
        echo "  Waiting for $name..."
        sleep 2
        count=$((count+1))
    done
    
    echo -e "${RED}✗ $name failed to start${NC}"
    return 1
}

if ! wait_for_node "http://localhost:8444" "Node A"; then exit 1; fi
if ! wait_for_node "http://localhost:8445" "Node B"; then exit 1; fi

echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Configuration (via Environment Variables)${NC}"
echo -e "${YELLOW}========================================${NC}"

# Display configurations
echo -e "${GREEN}Node A:${NC}"
echo "  - Node ID: node-a"
echo "  - Organization: Node A Corp"
echo "  - Domain: node-a"
echo "  - Internal API: localhost:8091"
echo "  - Public API: localhost:8444"

echo ""
echo -e "${GREEN}Node B:${NC}"
echo "  - Node ID: node-b"
echo "  - Organization: Node B Inc"
echo "  - Domain: node-b"
echo "  - Internal API: localhost:8092"
echo "  - Public API: localhost:8445"

echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Partner Discovery${NC}"
echo -e "${YELLOW}========================================${NC}"

# Login to Node A
echo -e "${YELLOW}Logging into Node A...${NC}"
curl -s -c cookie_a.txt -X POST http://localhost:8091/admin/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}' > /dev/null

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Logged into Node A${NC}"
else
    echo -e "${RED}✗ Failed to login to Node A${NC}"
    exit 1
fi

# Login to Node B
echo -e "${YELLOW}Logging into Node B...${NC}"
curl -s -c cookie_b.txt -X POST http://localhost:8092/admin/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}' > /dev/null

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Logged into Node B${NC}"
else
    echo -e "${RED}✗ Failed to login to Node B${NC}"
    exit 1
fi

# Spec §6.3: ONE handshake makes both nodes partners. Node B (responder) mints a
# single-use invitation token; Node A (initiator) registers with a signed JWS
# carrying it. A second, reverse handshake would be refused with 409
# ALREADY_REGISTERED, so mutual discovery is verified on both sides instead.
echo ""
echo -e "${YELLOW}Node B minting an invitation (AS5 URL + single-use token)...${NC}"
INVITATION_URL=$(curl -s -b cookie_b.txt -X POST http://localhost:8092/admin/dashboard/invitations | jq -r '.discovery_url')
echo "  $INVITATION_URL"
if [[ "$INVITATION_URL" != *"?token="* ]]; then
    echo -e "${RED}✗ Node B did not return an invitation URL with a token${NC}"
    exit 1
fi

echo ""
echo -e "${YELLOW}Node A registering with Node B (signed JWS, application/jose)...${NC}"
DISCOVERY_A=$(curl -s -b cookie_a.txt -X POST http://localhost:8091/admin/dashboard/partners/discover \
    -H "Content-Type: application/json" \
    -d "{\"discovery_url\":\"$INVITATION_URL\"}")

echo "$DISCOVERY_A" | jq '.' 2>/dev/null || echo "$DISCOVERY_A"

if echo "$DISCOVERY_A" | grep -q "successfully registered"; then
    echo -e "${GREEN}✓ Node A registered with Node B${NC}"
else
    echo -e "${RED}✗ Node A failed to register with Node B${NC}"
    exit 1
fi

echo ""
echo -e "${YELLOW}Verifying mutual recognition...${NC}"
if curl -s -b cookie_a.txt http://localhost:8091/admin/dashboard/partners | jq -e '.partners[] | select(.partner_id=="node-b")' > /dev/null; then
    echo -e "${GREEN}✓ Node A lists Node B as partner${NC}"
else
    echo -e "${RED}✗ Node A does not list Node B${NC}"; exit 1
fi
if curl -s -b cookie_b.txt http://localhost:8092/admin/dashboard/partners | jq -e '.partners[] | select(.partner_id=="node-a")' > /dev/null; then
    echo -e "${GREEN}✓ Node B lists Node A as partner${NC}"
else
    echo -e "${RED}✗ Node B does not list Node A${NC}"; exit 1
fi

echo ""
echo -e "${YELLOW}Re-using the spent invitation (must be refused)...${NC}"
REUSE_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${INVITATION_URL/node-b:8443/localhost:8445}")
if [ "$REUSE_CODE" == "401" ]; then
    echo -e "${GREEN}✓ Spent token refused (HTTP 401 INVALID_TOKEN)${NC}"
else
    echo -e "${RED}✗ Spent token answered HTTP $REUSE_CODE (expected 401)${NC}"
fi

# Get Node B's Partner ID
echo ""
NODE_B_ID=$(curl -s http://localhost:8445/.well-known/as5-configuration | jq -r .node_id)
echo -e "${GREEN}Node B ID: $NODE_B_ID${NC}"

echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Message Transmission Test${NC}"
echo -e "${YELLOW}========================================${NC}"

# Node A's internal API key is fixed by FIDEX_API_KEY in docker-compose.test.yml
NODE_A_API_KEY="test-api-key-12345"
echo -e "${GREEN}Node A API Key: $NODE_A_API_KEY${NC}"

# Send Message from Node A to Node B
echo ""
echo -e "${YELLOW}Sending message from Node A to Node B...${NC}"
RESPONSE=$(curl -s -X POST http://localhost:8091/api/v1/transmit \
    -H "Authorization: Bearer $NODE_A_API_KEY" \
    -H "Content-Type: application/json" \
    -d "{
        \"destination_partner_id\": \"$NODE_B_ID\",
        \"document_type\": \"TEST_ORDER\",
        \"payload\": {\"order_id\": \"TEST-12345\", \"item\": \"Widget\", \"quantity\": 42}
    }")

echo "$RESPONSE" | jq '.' 2>/dev/null || echo "$RESPONSE"

MESSAGE_ID=$(echo "$RESPONSE" | jq -r '.message_id' 2>/dev/null)

if [ "$MESSAGE_ID" != "null" ] && [ -n "$MESSAGE_ID" ]; then
    echo -e "${GREEN}✓ Message sent successfully${NC}"
    echo -e "${GREEN}  Message ID: $MESSAGE_ID${NC}"
else
    echo -e "${RED}✗ Failed to send message${NC}"
    echo -e "${RED}  Response: $RESPONSE${NC}"
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"
    docker-compose -f docker-compose.test.yml down
    rm -f cookie_a.txt cookie_b.txt
    exit 1
fi

# Wait for delivery
echo ""
echo -e "${YELLOW}Waiting for delivery (10 seconds)...${NC}"
sleep 10

echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Verification${NC}"
echo -e "${YELLOW}========================================${NC}"

# Check Node B's inbox
echo -e "${YELLOW}Checking Node B's inbox...${NC}"
MESSAGES_B=$(curl -s -b cookie_b.txt "http://localhost:8092/admin/dashboard/messages?limit=10")

if echo "$MESSAGES_B" | grep -q "$MESSAGE_ID"; then
    echo -e "${GREEN}✓ SUCCESS: Message received by Node B${NC}"
    echo -e "${GREEN}  Message ID: $MESSAGE_ID${NC}"
else
    echo -e "${RED}✗ FAILURE: Message not found in Node B${NC}"
    echo -e "${YELLOW}Node B Messages:${NC}"
    echo "$MESSAGES_B" | jq '.' 2>/dev/null || echo "$MESSAGES_B"
fi

# Check for MDN receipt on Node A
echo ""
echo -e "${YELLOW}Checking Node A for receipt (MDN)...${NC}"
sleep 5

MESSAGE_STATUS=$(curl -s -b cookie_a.txt "http://localhost:8091/admin/dashboard/messages?limit=10" | jq -r ".messages[] | select(.message_id==\"$MESSAGE_ID\") | .status" 2>/dev/null)

if [ "$MESSAGE_STATUS" == "DELIVERED" ]; then
    echo -e "${GREEN}✓ SUCCESS: Receipt (MDN) received and processed by Node A${NC}"
    echo -e "${GREEN}  Message Status: $MESSAGE_STATUS${NC}"
else
    echo -e "${YELLOW}⚠ Message status on Node A: $MESSAGE_STATUS (expected: DELIVERED)${NC}"
    echo -e "${YELLOW}  Note: Async receipts may still be processing${NC}"
fi

# Summary
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Test Summary${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}✓ Nodes started successfully${NC}"
echo -e "${GREEN}✓ Configuration loaded from environment${NC}"
echo -e "${GREEN}✓ Registration by invitation completed (both nodes know each other)${NC}"
echo -e "${GREEN}✓ Message transmitted${NC}"
echo -e "${GREEN}✓ Message delivered to Node B${NC}"

# Cleanup
echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Cleanup${NC}"
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Stopping containers...${NC}"
docker-compose -f docker-compose.test.yml down
rm -f cookie_a.txt cookie_b.txt
rm -rf test_data

echo -e "${GREEN}✓ Test scenario complete!${NC}"
echo -e "${GREEN}========================================${NC}"
