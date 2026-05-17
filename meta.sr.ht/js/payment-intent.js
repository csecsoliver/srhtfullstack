const stripe = Stripe(stripePubkey);

const darkMode = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
const appearance = {
	theme: darkMode ? 'night' : 'stripe',
};
const options = {
	layout: 'tabs',
	defaultValues: {
		billingDetails,
	},
};
const elements = stripe.elements({
	clientSecret: stripeClientSecret,
        appearance,
});
const paymentElement = elements.create('payment', options);
paymentElement.mount('#payment-details');

const form = document.querySelector("form");
form.addEventListener("submit", onSubmit);

async function onSubmit(ev) {
	ev.preventDefault();
	form.querySelector(".btn").disabled = true;

	const { error } = await stripe.confirmPayment({
		elements,
		confirmParams: {
			return_url,
		},
	});

	form.querySelector(".btn").disabled = false;
}
