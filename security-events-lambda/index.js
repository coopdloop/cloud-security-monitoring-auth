// security-events-lambda/index.js

exports.handler = async (event) => {
    console.log('Received event:', JSON.stringify(event, null, 2));

    // Process each record from SNS
    for (const record of event.Records) {
        try {
            const message = JSON.parse(record.Sns.Message);
            console.log('Processing message:', message);

            // Handle different event types
            switch (message.type) {
                case 'user.login':
                    console.log('Login event:', {
                        timestamp: message.data.timestamp,
                        email: message.data.email,
                        ip: message.data.ip_address
                    });
                    break;

                case 'session.revoked':
                    console.log('Session revoked:', {
                        userId: message.data.user_id,
                        sessionId: message.data.session_id
                    });
                    break;

                default:
                    console.log('Unknown event type:', message.type);
            }
        } catch (error) {
            console.error('Error processing record:', error);
        }
    }

    return {
        statusCode: 200,
        body: 'Events processed'
    };
};
