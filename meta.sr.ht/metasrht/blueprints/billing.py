import requests
from flask import Blueprint, render_template, request, redirect
from flask import url_for, abort, Response
from iso3166 import countries, countries_by_alpha2
from metasrht.graphql import Client, BillingAddressInput
from metasrht.graphql import Currency, PaymentInterval, PaymentStatus
from metasrht.graphql import PaymentIntentStatus, SetupIntentStatus
from metasrht.graphql import GraphQLClientGraphQLMultiError
from srht.app import session
from srht.config import cfg, get_origin
from srht.crypto import encrypt_request_authorization
from srht.graphql import Error, has_error
from srht.oauth import current_user, loginrequired
from srht.validation import Validation

billing = Blueprint('billing', __name__)
onboarding_redirect = cfg("meta.sr.ht::settings", "onboarding-redirect")

# Maximum number of invoices to display on abbreviated views
MAX_INVOICE = 4

CURRENCY_SYMBOLS = {
    Currency.EUR: "€",
    Currency.USD: "$",
}

CURRENCY_NAMES = {
    Currency.EUR: "Euro",
    Currency.USD: "US Dollar",
}

def get_address_input(valid):
    country = valid.require("country")
    if not country:
        return None

    country = countries_by_alpha2.get(country)
    valid.expect(country is not None, "Expected a valid country code")

    if not valid.ok:
        return None

    return BillingAddressInput(**{
        "fullName": valid.optional("full_name") or None,
        "businessName": valid.optional("business_name") or None,
        "address1": valid.optional("address1") or None,
        "address2": valid.optional("address2") or None,
        "city": valid.optional("city") or None,
        "region": valid.optional("region") or None,
        "postcode": valid.optional("postcode") or None,
        "country": country,
        "vat": valid.optional("vat") or None,
    })

@billing.route("/billing")
@loginrequired
def billing_GET():
    session.pop("idempotency_key", None)

    user = Client().get_billing_dashboard().me
    if user.subscription is None:
        return redirect(url_for(".billing_setup_GET"))

    if len(user.invoices.results) > MAX_INVOICE:
        user.invoices.results = user.invoices.results[:MAX_INVOICE]

    return render_template("billing.html",
            message=session.pop("message", None), user=user,
            sym=CURRENCY_SYMBOLS[user.subscription.currency])

@billing.route("/billing/address")
@loginrequired
def billing_address_GET():
    address = Client().get_billing_address().me.billing_address
    return render_template("billing-edit-address.html",
        address=address, countries=countries)

@billing.route("/billing/address", methods=["POST"])
@loginrequired
def billing_address_POST():
    client = Client()
    valid = Validation(request)
    address_input = get_address_input(valid)

    if not valid.ok:
        address = client.get_billing_address().me.billing_address
        return render_template("billing-edit-address.html",
            address=address, countries=countries, valid=valid)

    with valid:
        client.update_billing_address(address_input)

    if not valid.ok:
        address = client.get_billing_address().me.billing_address
        return render_template("billing-edit-address.html",
            address=address, countries=countries, valid=valid)

    return redirect(url_for("billing.billing_GET"))

def billing_setup_render(client, valid=None):
    setup = client.get_billing_setup()
    user, products = setup.user, setup.products

    if user.payment_status not in [
            PaymentStatus.UNPAID,
            PaymentStatus.FREE,
            PaymentStatus.SUBSIDIZED,
        ]:
        return redirect(url_for(".billing_GET"))

    currencies = set()
    for product in products:
        currencies.update(p.currency for p in product.prices)

    return render_template("billing-setup.html",
        user=user, products=products,
        countries=countries,
        currencies=sorted(currencies),
        CURRENCY_NAMES=CURRENCY_NAMES,
        CURRENCY_SYMBOLS=CURRENCY_SYMBOLS,
        message=session.pop("message", None),
        **({"valid": valid} if valid else {}))

@billing.route("/billing/setup")
@loginrequired
def billing_setup_GET():
    session.pop("idempotency_key", None)
    return billing_setup_render(Client())

@billing.route("/billing/setup", methods=["POST"])
@loginrequired
def billing_setup_POST():
    client = Client()
    valid = Validation(request)

    idempotency_key = session.get("idempotency_key", None)
    address_input = get_address_input(valid)
    product_id = valid.require("product_id")
    interval = valid.require("interval", cls=PaymentInterval)
    currency = valid.require("currency", cls=Currency)
    with valid.catch(ValueError, "Invalid product ID", field="product_id"):
        product_id = int(product_id) if product_id else None

    if not valid.ok:
        return billing_setup_render(client, valid)

    with valid:
        intent = client.order_product(product_id, currency,
            address_input, interval, idempotency_key).intent

    if not valid.ok:
        return billing_setup_render(client, valid)

    currency = intent.subscription.currency
    session["idempotency_key"] = intent.idempotency_key
    return render_template("billing-payment-intent.html",
        intent=intent, sym=CURRENCY_SYMBOLS[currency])

@billing.route("/billing/validate-payment")
@loginrequired
def validate_payment_GET():
    intent_id = request.args.get("payment_intent")
    client = Client()

    try:
        outcome = client.finalize_payment_intent(intent_id).intent.outcome
    except GraphQLClientGraphQLMultiError as err:
        # finalizePaymentIntent is non-idempotent. If we hit NOT_FOUND we could
        # have already processed the payment successfully.
        if not has_error(err, Error.NOT_FOUND):
            raise
        sub = client.get_payment_outcome().me.subscription
        if not sub:
            abort(404)
        outcome = sub.payment

    match outcome.status:
        case PaymentIntentStatus.SUCCEEDED | PaymentIntentStatus.PROCESSING:
            return redirect(url_for("billing.billing_complete"))
        case _:
            session["message"] = 'We were unable to complete your payment. Please try again, or contact support for assistance.'
            return redirect(url_for("billing.billing_setup_GET"))

@billing.route("/billing/complete")
@loginrequired
def billing_complete():
    return render_template("billing-complete.html",
            onboarding_redirect=onboarding_redirect)

@billing.route("/billing/new-payment-method")
@loginrequired
def new_payment_method_GET():
    client = Client()
    user = client.get_setup_intent_detail().me
    if not user.subscription:
        return redirect(url_for(".billing_setup_GET"))

    if user.payment_status != PaymentStatus.DELINQUENT:
        intent = client.create_setup_intent().intent
        return render_template("billing-setup-intent.html",
            intent=intent,
            sub=user.subscription,
            address=user.billing_address)
    else:
        idempotency_key = session.get("idempotency_key", None)

        # When adding a payment method to a delinquent account, we use a
        # payment intent instead of a setup intent to charge the user
        # immediately
        intent = client.create_renewal_payment_intent(
            user.subscription.id, idempotency_key).intent

        currency = intent.subscription.currency
        session["idempotency_key"] = intent.idempotency_key
        return render_template("billing-payment-intent.html",
            intent=intent, sym=CURRENCY_SYMBOLS[currency])

@billing.route("/billing/validate-setup")
@loginrequired
def validate_setup_GET():
    intent_id = request.args.get("setup_intent")
    status = Client().finalize_setup_intent(intent_id).result.status

    match status:
        case SetupIntentStatus.SUCCEEDED:
            session["message"] = f"Your new payment method was added successfully."
        case SetupIntentStatus.CANCELLED:
            session["message"] = f"Adding your new payment method was cancelled before it could be completed."

    return redirect(url_for("billing.billing_GET"))

@billing.route("/billing/remove-method/<method_id>", methods=["POST"])
@loginrequired
def remove_method_POST(method_id):
    Client().remove_payment_method(method_id)
    return redirect(url_for("billing.billing_GET"))

@billing.route("/billing/set-default-method/<method_id>", methods=["POST"])
@loginrequired
def set_default_method_POST(method_id):
    Client().set_default_payment_method(method_id)
    return redirect(url_for("billing.billing_GET"))

def change_product_render(**kwargs):
    resp = Client().get_product_selection()
    user, products = resp.me, resp.products

    if user.payment_status not in [
            PaymentStatus.CURRENT,
            PaymentStatus.DELINQUENT,
    ]:
        return redirect(url_for(".billing_GET"))

    return render_template("billing-change-product.html",
        products=products, user=user, **kwargs)

@billing.route("/billing/change-product")
@loginrequired
def change_product_GET():
    return change_product_render()

@billing.route("/billing/change-product", methods=["POST"])
@loginrequired
def change_product_POST():
    valid = Validation(request)
    product_id = valid.require("product_id")
    interval = valid.require("interval", cls=PaymentInterval)
    with valid.catch(ValueError, "Invalid product ID", field="product_id"):
        product_id = int(product_id) if product_id else None
    if not valid.ok:
        return change_product_render(valid=valid)

    client = Client()
    params = client.get_subscription_parameters().me
    status = params.payment_status
    sub_id = params.subscription.id
    client.change_product(sub_id, product_id, interval)

    if status == PaymentStatus.DELINQUENT:
        return redirect(url_for(".new_payment_method_GET"))
    return redirect(url_for(".billing_GET"))

@billing.route("/billing/cancel")
@loginrequired
def cancel_GET():
    user = Client().get_cancel_parameters().me
    if user.payment_status not in [PaymentStatus.CURRENT, PaymentStatus.DELINQUENT]:
        return redirect(url_for(".billing_GET"))
    return render_template("billing-cancel.html", user=user)

@billing.route("/billing/cancel", methods=["POST"])
@loginrequired
def cancel_POST():
    client = Client()
    params = client.get_subscription_parameters().me
    client.cancel_subscription(params.subscription.id)
    return redirect(url_for("billing.cancelled_GET"))

@billing.route("/billing/cancelled")
@loginrequired
def cancelled_GET():
    user = Client().get_subscription_parameters().me
    return render_template("billing-cancelled.html", user=user)

@billing.route("/billing/invoices")
@loginrequired
def invoices_GET():
    cursor = request.args.get("next")
    conn = Client().get_invoices(cursor).me.invoices
    cursor, invoices = conn.cursor, conn.results
    return render_template("billing-invoices.html",
        invoices=invoices, cursor=cursor)

@billing.route("/billing/invoice/<int:invoice_id>", methods=["POST"])
@loginrequired
def invoice_POST(invoice_id):
    origin = cfg("meta.sr.ht", "api-origin", default=get_origin("meta.sr.ht"))
    headers = {
        "X-Forwarded-For": ", ".join(request.access_route),
        **encrypt_request_authorization(user=current_user),
    }
    r = requests.post(f"{origin}/query/invoice/{invoice_id}",
        headers=headers, data=request.form)
    headers = [('Content-Disposition', r.headers["Content-Disposition"])]
    return Response(r.content, mimetype="application/pdf", headers=headers)
