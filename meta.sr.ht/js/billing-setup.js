Array.from(document.querySelectorAll(".noscript-hidden"))
        .map(node => node.classList.remove("noscript-hidden"));

const symbols = {
        "EUR": "€",
        "USD": "$",
};
selectCurrency(currency);

Array.from(document.querySelectorAll("#currency")).map(selector => {
	selector.addEventListener("click", onSelectCurrency);
});

Array.from(document.querySelectorAll(".select-product")).map(btn => {
	btn.addEventListener("click", onSelectProduct);
});

document.getElementById("show-subsidized").addEventListener("click", ev => {
	ev.stopPropagation();
	ev.preventDefault();
	document.querySelector(".products").classList.add("show-subsidized");
	document.querySelector(".products").classList.remove("hide-subsidized");
});

document.getElementById("hide-subsidized").addEventListener("click", ev => {
	ev.stopPropagation();
	ev.preventDefault();
	document.querySelector(".products").classList.add("hide-subsidized");
	document.querySelector(".products").classList.remove("show-subsidized");
});

function onSelectCurrency(ev) {
	ev.stopPropagation();
	ev.preventDefault();
	selectCurrency(ev.target.value);
}

function onSelectProduct(ev) {
	ev.stopPropagation();
	ev.preventDefault();

	const parentNode = ev.target.parentElement;
	console.assert(parentNode.classList.contains("product"));
	const productid = parseInt(parentNode.dataset.productid);
	const product = products.filter(p => p.id === productid)[0];
	selectProduct(product);
}

function selectCurrency(cur) {
	currency = cur;
	Array.from(document.querySelectorAll(".price")).map(pr => {
		const productID = parseInt(pr.dataset.productid);
		const product = products.filter(p => p.id === productID)[0];
		const price = product.prices[currency];
		const monthly = (price / 100).toFixed(0);
		const sym = symbols[currency];
		pr.textContent = `${sym}${monthly}/month`;
	});
}

function selectProduct(product) {
	Array.from(document.querySelectorAll(`.product:not(#product-${product.id})`)).
		map(p => p.classList.add("d-none"));
	Array.from(document.querySelectorAll(`.hide-after-selection`)).
		map(p => p.classList.add("d-none"));
	Array.from(document.querySelectorAll(`.show-after-selection`)).
		map(p => p.classList.remove("d-none"));

	const p = document.getElementById(`product-${product.id}`);
	const btn = p.querySelector(".btn");
	btn.classList.remove("btn-primary");
	btn.classList.add("btn-success");
	btn.innerText = "✓ Selected";

	// XXX: It would be nice if the GraphQL API calculated the pricing for
	// different payment intervals so that we don't have to replicate that
	// here.
	const price = product.prices[currency];
	console.log(product, currency, price);
	const monthly = (price / 100).toFixed(0);
	const annually = (price * 10 / 100).toFixed(0);

	document.getElementById("price-monthly").innerText =
		`${symbols[currency]}${monthly}`;
	document.getElementById("price-annually").innerText =
		`${symbols[currency]}${annually}`;

	document.getElementById("product_id").value = product.id;
}
